package list

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/intbounds"
	"github.com/emersion/go-imap/v2"
	"xorm.io/xorm"
)

const searchBatch = 400

// SearchRows acts only on links from the selected mailbox's visible snapshot.
// Fetch each candidate's necessary columns at most once per batch, even for
// nested OR/NOT, and never return unfiltered data on a failed query.
func SearchRows(ctx *context.Context, rows []*response.UserEmailUIDData, c *imap.SearchCriteria) ([]*response.UserEmailUIDData, error) {
	if c == nil || len(rows) == 0 {
		return rows, nil
	}
	var maxUID, maxSeq uint32
	for _, row := range rows {
		u, ok1 := intbounds.Uint32(int64(row.ID))
		n, ok2 := intbounds.Uint32(int64(row.SerialNumber))
		if !ok1 || !ok2 || u == 0 || n == 0 {
			return nil, errors.New("invalid search snapshot identifiers")
		}
		maxUID = max(maxUID, u)
		maxSeq = max(maxSeq, n)
	}
	return SearchSnapshot(ctx, rows, c, maxUID, maxSeq)
}

func SearchSnapshot(ctx *context.Context, rows []*response.UserEmailUIDData, c *imap.SearchCriteria, maxUID, maxSeq uint32) ([]*response.UserEmailUIDData, error) {
	if c == nil || len(rows) == 0 {
		return rows, nil
	}
	wanted := map[string]bool{}
	neededColumns(c, wanted)
	if len(wanted) == 0 {
		out := make([]*response.UserEmailUIDData, 0, len(rows))
		for _, row := range rows {
			if matchSearch(row, nil, c, maxUID, maxSeq) {
				out = append(out, row)
			}
		}
		return out, nil
	}
	columns := []string{"id"}
	for col := range wanted {
		columns = append(columns, col)
	}
	slices.Sort(columns)
	out := make([]*response.UserEmailUIDData, 0)
	batchSize := searchBatch
	if wanted["text"] || wanted["html"] {
		batchSize = 4
	} // bound allocations for large MIME parts
	for offset := 0; offset < len(rows); offset += batchSize {
		batch := rows[offset:min(offset+batchSize, len(rows))]
		ids := make([]int, 0, len(batch))
		for _, row := range batch {
			if matchLink(row, c, maxUID, maxSeq) {
				ids = append(ids, row.EmailID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		q := db.Instance.NewSession()
		if ctx != nil && ctx.Context != nil {
			q.Context(ctx.Context)
		}
		q.Cols(columns...).In("id", ids)
		for _, h := range c.Header {
			cols := headerColumns(h.Key)
			if len(cols) > 0 {
				applyASCIIPrefilter(q, h.Value, cols...)
			}
		}
		for _, value := range c.Body {
			applyASCIIPrefilter(q, value, "text", "html")
		}
		for _, value := range c.Text {
			applyASCIIPrefilter(q, value, "subject", "from_address", "from_name", "to", "cc", "bcc", "text", "html")
		}
		if c.Larger > 0 {
			q.Where("size>?", c.Larger)
		}
		if c.Smaller > 0 {
			q.Where("size<?", c.Smaller)
		}
		var emails []models.Email
		err := q.Find(&emails)
		q.Close()
		if err != nil {
			return nil, err
		}
		data := make(map[int]*models.Email, len(emails))
		for i := range emails {
			data[emails[i].Id] = &emails[i]
		}
		for _, row := range batch {
			if e := data[row.EmailID]; e != nil && matchSearch(row, e, c, maxUID, maxSeq) {
				out = append(out, row)
			}
		}
	}
	return out, nil
}

var escapeSearchLike = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")

// Only narrow top-level conjunctions. Unmatched non-ASCII rows are retained
// for exact Go Unicode folding; SQL collation is not the authority for SEARCH.
func applyASCIIPrefilter(q *xorm.Session, value string, columns ...string) {
	if value == "" || len(value) > 256 || !ascii(value) {
		return
	}
	var clauses []string
	var args []any
	pattern := "%" + escapeSearchLike.Replace(strings.ToLower(value)) + "%"
	for _, column := range columns {
		col := db.Instance.Quote(column)
		nonASCII := fmt.Sprintf("length(CAST(%s AS BLOB)) <> length(%s)", col, col)
		fold := col
		switch db.Instance.DriverName() {
		case "postgres":
			nonASCII = fmt.Sprintf("octet_length(%s) <> char_length(%s)", col, col)
			fold = col + ` COLLATE "C"`
		case "mysql":
			nonASCII = fmt.Sprintf("octet_length(%s) <> char_length(%s)", col, col)
			fold = "CONVERT(" + col + " USING utf8mb4) COLLATE utf8mb4_bin"
		}
		clauses = append(clauses, fmt.Sprintf("(LOWER(%s) LIKE ? ESCAPE '!' OR %s)", fold, nonASCII))
		args = append(args, pattern)
	}
	q.Where("("+strings.Join(clauses, " OR ")+")", args...)
}

func headerColumns(key string) []string {
	switch k := strings.ToLower(key); k {
	case "from":
		return []string{"from_name", "from_address"}
	case "reply-to":
		return []string{"reply_to"}
	case "subject", "to", "cc", "bcc", "sender":
		return []string{k}
	}
	return nil
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 128 || s[i] == 0 {
			return false
		}
	}
	return true
}

func neededColumns(c *imap.SearchCriteria, cols map[string]bool) {
	if !c.Since.IsZero() || !c.Before.IsZero() {
		cols["create_time"] = true
	}
	if !c.SentSince.IsZero() || !c.SentBefore.IsZero() {
		cols["send_date"] = true
	}
	if c.Larger > 0 || c.Smaller > 0 {
		cols["size"] = true
	}
	for _, h := range c.Header {
		for _, col := range headerColumns(h.Key) {
			cols[col] = true
		}
	}
	if len(c.Body) > 0 || len(c.Text) > 0 {
		cols["text"] = true
		cols["html"] = true
	}
	if len(c.Text) > 0 {
		for _, col := range []string{"subject", "from_name", "from_address", "to", "cc", "bcc"} {
			cols[col] = true
		}
	}
	for i := range c.Not {
		neededColumns(&c.Not[i], cols)
	}
	for i := range c.Or {
		neededColumns(&c.Or[i][0], cols)
		neededColumns(&c.Or[i][1], cols)
	}
}

func boundedRange(n, start, stop, highest uint32) bool {
	if start == 0 {
		start = highest
	}
	if stop == 0 {
		stop = highest
	}
	return n >= min(start, stop) && n <= max(start, stop)
}

func matchLink(row *response.UserEmailUIDData, c *imap.SearchCriteria, maxUID, maxSeq uint32) bool {
	uid, ok1 := intbounds.Uint32(int64(row.ID))
	seq, ok2 := intbounds.Uint32(int64(row.SerialNumber))
	if !ok1 || !ok2 || uid == 0 || seq == 0 {
		return false
	}
	for _, set := range c.UID {
		if !slices.ContainsFunc(set, func(r imap.UIDRange) bool {
			return boundedRange(uid, uint32(r.Start), uint32(r.Stop), maxUID)
		}) {
			return false
		}
	}
	for _, set := range c.SeqNum {
		if !slices.ContainsFunc(set, func(r imap.SeqRange) bool { return boundedRange(seq, r.Start, r.Stop, maxSeq) }) {
			return false
		}
	}
	for _, flag := range c.Flag {
		if !hasFlag(row, flag) {
			return false
		}
	}
	for _, flag := range c.NotFlag {
		if hasFlag(row, flag) {
			return false
		}
	}
	return true
}

func hasFlag(row *response.UserEmailUIDData, flag imap.Flag) bool {
	switch flag {
	case imap.FlagSeen:
		return row.IsRead == 1
	case imap.FlagDeleted:
		return row.IMAPDeleted
	case imap.FlagFlagged:
		return row.IsStarred
	case imap.FlagDraft:
		return row.Status == 4
	case imap.FlagJunk:
		return row.Status == 5
	}
	return false
}

func matchSearch(row *response.UserEmailUIDData, e *models.Email, c *imap.SearchCriteria, maxUID, maxSeq uint32) bool {
	if !matchLink(row, c, maxUID, maxSeq) {
		return false
	}
	if e != nil {
		if !matchesEmailCriteria(e, c) {
			return false
		}
	} else if len(c.Header) > 0 {
		return false
	}
	for i := range c.Not {
		if matchSearch(row, e, &c.Not[i], maxUID, maxSeq) {
			return false
		}
	}
	for i := range c.Or {
		if !matchSearch(row, e, &c.Or[i][0], maxUID, maxSeq) && !matchSearch(row, e, &c.Or[i][1], maxUID, maxSeq) {
			return false
		}
	}
	return true
}

// matchesEmailCriteria checks if an email matches the search criteria
func matchesEmailCriteria(email *models.Email, criteria *imap.SearchCriteria) bool {
	// Date filters (internal date = CreateTime)
	if !criteria.Since.IsZero() {
		if email.CreateTime.Before(truncateToDate(criteria.Since)) {
			return false
		}
	}
	if !criteria.Before.IsZero() {
		if !email.CreateTime.Before(truncateToDate(criteria.Before)) {
			return false
		}
	}

	// Sent date filters
	if !criteria.SentSince.IsZero() {
		if email.SendDate.Before(truncateToDate(criteria.SentSince)) {
			return false
		}
	}
	if !criteria.SentBefore.IsZero() {
		if !email.SendDate.Before(truncateToDate(criteria.SentBefore)) {
			return false
		}
	}

	// Size filters
	if criteria.Larger > 0 {
		if int64(email.Size) <= criteria.Larger {
			return false
		}
	}
	if criteria.Smaller > 0 {
		if int64(email.Size) >= criteria.Smaller {
			return false
		}
	}

	// Header field search
	for _, hf := range criteria.Header {
		if !matchesHeader(email, hf.Key, hf.Value) {
			return false
		}
	}

	// Body search
	for _, pattern := range criteria.Body {
		if !matchesBody(email, pattern) {
			return false
		}
	}

	// Text search (headers + body)
	for _, pattern := range criteria.Text {
		if !matchesText(email, pattern) {
			return false
		}
	}

	return true
}

// truncateToDate removes time component from a time.Time
func truncateToDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// matchesHeader checks if an email matches a header field search
func matchesHeader(email *models.Email, key, value string) bool {
	key = strings.ToLower(key)
	value = strings.ToLower(value)

	switch key {
	case "subject":
		return strings.Contains(strings.ToLower(email.Subject), value)
	case "from":
		return strings.Contains(strings.ToLower(email.FromAddress), value) ||
			strings.Contains(strings.ToLower(email.FromName), value)
	case "to":
		return strings.Contains(strings.ToLower(email.To), value)
	case "cc":
		return strings.Contains(strings.ToLower(email.Cc), value)
	case "bcc":
		return strings.Contains(strings.ToLower(email.Bcc), value)
	case "reply-to":
		return strings.Contains(strings.ToLower(email.ReplyTo), value)
	case "sender":
		return strings.Contains(strings.ToLower(email.Sender), value)
	default:
		// For unknown headers, we can't match
		return false
	}
}

// matchesBody checks if an email body matches the pattern
func matchesBody(email *models.Email, pattern string) bool {
	pattern = strings.ToLower(pattern)

	// Check text body
	if email.Text.Valid && strings.Contains(strings.ToLower(email.Text.String), pattern) {
		return true
	}

	// Check HTML body
	if email.Html.Valid && strings.Contains(strings.ToLower(email.Html.String), pattern) {
		return true
	}

	return false
}

// matchesText checks if an email (headers + body) matches the pattern
func matchesText(email *models.Email, pattern string) bool {
	pattern = strings.ToLower(pattern)

	// Check headers
	if strings.Contains(strings.ToLower(email.Subject), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(email.FromAddress), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(email.FromName), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(email.To), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(email.Cc), pattern) {
		return true
	}
	if strings.Contains(strings.ToLower(email.Bcc), pattern) {
		return true
	}

	// Check body
	return matchesBody(email, pattern)
}
