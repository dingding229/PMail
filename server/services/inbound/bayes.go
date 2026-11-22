package inbound

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

const tokenLimit = 2000
const tokenBodyLimit = 64 << 10
const MinSamples = 20
const MaxMarkBatch = 50

// Classify on message-presence counts, not term frequency. Hash tokens before
// storage and cap the per-message vocabulary to bound computation and writes.
func tokens(subject, body string) []string {
	text := subject + "\n" + body
	seen := map[string]bool{}
	var result []string
	add := func(s string) {
		if !seen[s] && len(result) < tokenLimit {
			seen[s] = true
			result = append(result, hashValue(s))
		}
	}
	var prev rune
	var word strings.Builder
	flush := func() {
		if word.Len() >= 2 && word.Len() <= 48 {
			add(word.String())
		}
		word.Reset()
	}
	for _, r := range text {
		if len(result) >= tokenLimit {
			break
		}
		if unicode.Is(unicode.Han, r) {
			flush()
			if prev != 0 {
				add(string([]rune{prev, r}))
			}
			prev = r
			continue
		}
		prev = 0
		r = unicode.ToLower(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if word.Len() < 49 {
				word.WriteRune(r)
			}
		} else {
			flush()
		}
	}
	flush()
	slices.Sort(result)
	return result
}

func bayesScore(ctx context.Context, engine *xorm.Engine, id int, subject, body string) (float64, error) {
	var totals models.SpamTotals
	ok, err := engine.Context(ctx).ID(id).Get(&totals)
	if err != nil || !ok || totals.Spam < MinSamples || totals.Ham < MinSamples {
		return 0, err
	}
	keys := tokens(subject, body)
	var odds []float64
	for i := 0; i < len(keys); i += 250 {
		var rows []models.SpamToken
		if err = engine.Context(ctx).Where("user_id=?", id).In("token_key", keys[i:min(i+250, len(keys))]).Find(&rows); err != nil {
			return 0, err
		}
		for _, token := range rows {
			if token.Spam+token.Ham < 2 {
				continue
			}
			sp := (float64(token.Spam) + 1) / (float64(totals.Spam) + 2)
			ha := (float64(token.Ham) + 1) / (float64(totals.Ham) + 2)
			odds = append(odds, math.Log(sp/ha))
		}
	}
	slices.SortFunc(odds, func(a, b float64) int {
		if d := math.Abs(b) - math.Abs(a); d < 0 {
			return -1
		} else if d > 0 {
			return 1
		}
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	var sum float64
	for _, v := range odds[:min(15, len(odds))] {
		sum += v
	}
	result := math.Round(5*math.Tanh(sum/8)*100) / 100
	return result, nil
}

func updateTokens(tx *xorm.Session, id int, keys []string, spam bool, delta int64) error {
	if len(keys) == 0 {
		return nil
	}
	if delta < 0 {
		column := "ham"
		if spam {
			column = "spam"
		}
		for i := 0; i < len(keys); i += 200 {
			part := keys[i:min(i+200, len(keys))]
			marks := strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")
			args := []any{id}
			for _, k := range part {
				args = append(args, k)
			}
			// Reversal never decrements an unknown token below zero.
			if _, err := tx.Exec(append([]any{"UPDATE spam_token SET " + column + "=" + column + "-1 WHERE user_id=? AND " + column + ">0 AND token_key IN (" + marks + ")"}, args...)...); err != nil {
				return err
			}
			if _, err := tx.Exec(append([]any{"DELETE FROM spam_token WHERE user_id=? AND spam=0 AND ham=0 AND token_key IN (" + marks + ")"}, args...)...); err != nil {
				return err
			}
		}
		return nil
	}
	var a, b int64
	if spam {
		a = delta
	} else {
		b = delta
	}
	for i := 0; i < len(keys); i += 200 {
		part := keys[i:min(i+200, len(keys))]
		args := make([]any, 0, len(part)*4)
		for _, key := range part {
			args = append(args, id, key, a, b)
		}
		query := "INSERT INTO spam_token (user_id,token_key,spam,ham) VALUES " + strings.TrimSuffix(strings.Repeat("(?,?,?,?),", len(part)), ",")
		if db.Instance.DriverName() == config.DBTypeMySQL {
			query += " ON DUPLICATE KEY UPDATE spam=spam+VALUES(spam),ham=ham+VALUES(ham)"
		} else {
			query += " ON CONFLICT (user_id,token_key) DO UPDATE SET spam=spam_token.spam+excluded.spam,ham=spam_token.ham+excluded.ham"
		}
		if _, err := tx.Exec(append([]any{query}, args...)...); err != nil {
			return err
		}
	}
	return nil
}
func updateTotal(tx *xorm.Session, id int, spam bool, delta int64) error {
	var t models.SpamTotals
	ok, err := tx.ID(id).Get(&t)
	if err != nil {
		return err
	}
	if spam {
		t.Spam = max(0, t.Spam+delta)
	} else {
		t.Ham = max(0, t.Ham+delta)
	}
	t.UserID = id
	if !ok {
		_, err = tx.Insert(&t)
	} else {
		_, err = tx.ID(id).Cols("spam", "ham").Update(&t)
	}
	return err
}

func train(tx *xorm.Session, owner, id int, spam, enabled bool, keys []string) error {
	var old models.SpamTraining
	found, err := tx.Where("user_id=? AND email_id=?", owner, id).Get(&old)
	if err != nil {
		return err
	}
	if found && old.IsSpam == spam {
		return nil
	}
	if found {
		var previous []string
		if err = json.Unmarshal([]byte(old.Tokens), &previous); err != nil || len(previous) > tokenLimit {
			return fmt.Errorf("invalid stored spam training")
		}
		if err = updateTokens(tx, owner, previous, old.IsSpam, -1); err != nil {
			return err
		}
		if err = updateTotal(tx, owner, old.IsSpam, -1); err != nil {
			return err
		}
	}
	if !enabled {
		if found {
			_, err = tx.Where("user_id=? AND email_id=?", owner, id).Delete(&models.SpamTraining{})
		}
		return err
	}
	if err = updateTokens(tx, owner, keys, spam, 1); err != nil {
		return err
	}
	if err = updateTotal(tx, owner, spam, 1); err != nil {
		return err
	}
	sample, _ := json.Marshal(keys)
	row := models.SpamTraining{UserID: owner, EmailID: id, IsSpam: spam, Tokens: string(sample)}
	if found {
		_, err = tx.Where("user_id=? AND email_id=?", owner, id).Cols("is_spam", "tokens").Update(&row)
	} else {
		_, err = tx.Insert(&row)
	}
	return err
}

// MarkSpam only affects the caller's links. Validation and training are in
// one owner-locked transaction, so duplicates and overlapping requests agree.
func MarkSpam(ctx context.Context, owner int, ids []int, isSpam bool) error {
	if len(ids) == 0 || len(ids) > MaxMarkBatch {
		return ErrInvalid
	}
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	if sorted[0] <= 0 {
		return ErrInvalid
	}
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockOwner(tx, owner); err != nil {
			return err
		}
		p := models.SpamPreference{BayesEnabled: true}
		var pref models.SpamPreference
		if ok, err := tx.ID(owner).Get(&pref); err != nil {
			return err
		} else if ok {
			p = pref
		}
		for _, id := range sorted {
			var link models.UserEmail
			ok, err := tx.Where("user_id=? AND email_id=? AND status<>?", owner, id, consts.EmailStatusDrafts).Asc("id").Cols("id", "inbound_spam").Get(&link)
			if err != nil {
				return err
			}
			if !ok {
				return ErrNotFound
			}
			var row models.Email
			ok, err = tx.ID(id).Where("type=? AND status<>?", consts.EmailTypeReceive, consts.EmailStatusDrafts).Select("id,subject,SUBSTR(text,1,65536) AS text").Get(&row)
			if err != nil {
				return err
			}
			if !ok {
				return ErrNotFound
			}
			var keys []string
			if p.BayesEnabled {
				keys = tokens(string(clippedBytes([]byte(row.Subject), 1024)), string(clippedBytes([]byte(row.Text.String), tokenBodyLimit)))
			}
			if err = train(tx, owner, id, isSpam, p.BayesEnabled, keys); err != nil {
				return err
			}
			personal := ReadDetail(models.Email{}.InboundAnalysis, link.InboundSpam).Spam
			if personal == nil {
				personal = &Spam{Reasons: []Reason{}}
			}
			personal.Reasons = slices.DeleteFunc(personal.Reasons, func(r Reason) bool { return r.Code == "manual_ham" || r.Code == "manual_spam" })
			status := consts.EmailStatusWait
			verdict, reason := "ham", "manual_ham"
			if isSpam {
				status = consts.EmailStatusJunk
				verdict, reason = "spam", "manual_spam"
			}
			personal.Verdict = verdict
			personal.Reasons = append(personal.Reasons, Reason{Code: reason})
			_, err = tx.Table(&models.UserEmail{}).Where("user_id=? AND email_id=? AND status<>?", owner, id, consts.EmailStatusDrafts).Update(map[string]any{"status": status, "group_id": 0, "inbound_spam": Storage(personal).String})
			if err != nil {
				return err
			}
		}
		return nil
	})
}
