package compose

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"golang.org/x/net/html"
	"xorm.io/xorm"
)

const MaxImage = 10 << 20
const ImageQuota = 50 << 20
const MaxRefs = 20
const ImageLife = 24 * time.Hour

var ErrTooLarge = errors.New("inline_image_too_large")
var ErrType = errors.New("inline_image_type")
var ErrQuota = errors.New("inline_image_quota")
var ErrRef = errors.New("inline_image_invalid_reference")
var ErrNotFound = errors.New("compose_not_found")
var ErrSignature = errors.New("signature_invalid")
var ErrSignatureLimit = errors.New("signature_limit")
var cidFormat = regexp.MustCompile(`^[0-9a-f]{32}@pmail\.inline$`)

type Ref struct {
	CID string `json:"cid"`
}
type Uploaded struct {
	CID         string `json:"cid"`
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

func raster(data []byte) string {
	switch ct := http.DetectContentType(data); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return ct
	}
	return ""
}
func filename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	n := 0
	for _, c := range name {
		if n >= 180 {
			break
		}
		if !unicode.IsControl(c) && !unicode.In(c, unicode.Cf) {
			b.WriteRune(c)
			n++
		}
	}
	if b.Len() == 0 || b.String() == "." || b.String() == ".." || b.String() == "/" {
		return "image"
	}
	return b.String()
}
func Upload(ctx context.Context, uid int, name string, reader io.Reader) (Uploaded, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxImage+1))
	if err != nil {
		return Uploaded{}, err
	}
	if len(data) > MaxImage {
		return Uploaded{}, ErrTooLarge
	}
	ct := raster(data)
	if ct == "" {
		return Uploaded{}, ErrType
	}
	var u models.ComposeImage
	randBytes := make([]byte, 16)
	if _, err = rand.Read(randBytes); err != nil {
		return Uploaded{}, err
	}
	u.CID = hex.EncodeToString(randBytes) + "@pmail.inline"
	u.Filename = filename(name)
	u.UserID = uid
	u.ContentType = ct
	u.Size = len(data)
	u.Content = data
	u.Touched = time.Now().Unix()
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if _, err := quota.Lock(tx, uid); err != nil {
			return err
		}
		var sum int64
		if _, err := tx.SQL("SELECT COALESCE(SUM(size),0) FROM compose_image WHERE user_id=?", uid).Get(&sum); err != nil {
			return err
		}
		if sum+int64(u.Size) > ImageQuota {
			return ErrQuota
		}
		_, err := tx.Insert(&u)
		return err
	})
	if err != nil {
		return Uploaded{}, err
	}
	return Uploaded{u.CID, "/api/compose/inline-image/" + u.CID, u.Filename, u.ContentType, u.Size}, nil
}
func Preview(ctx context.Context, uid int, cid string, head bool) (models.ComposeImage, error) {
	var u models.ComposeImage
	if !cidFormat.MatchString(cid) {
		return u, ErrNotFound
	}
	q := db.Instance.Context(ctx).Where("user_id=? AND cid=? AND (touched>=? OR EXISTS (SELECT 1 FROM draft_inline_image WHERE draft_inline_image.cid=compose_image.cid))", uid, cid, time.Now().Add(-ImageLife).Unix())
	if head {
		q.Omit("content")
	}
	ok, err := q.Get(&u)
	if err != nil {
		return u, err
	}
	if !ok {
		return u, ErrNotFound
	}
	return u, nil
}
func sourceCIDs(raw string) []string {
	var ids []string
	z := html.NewTokenizer(strings.NewReader(raw))
	for k := z.Next(); k != html.ErrorToken; k = z.Next() {
		if k != html.StartTagToken && k != html.SelfClosingTagToken {
			continue
		}
		tag, attrs := z.TagName()
		if string(tag) != "img" {
			continue
		}
		for attrs {
			k, v, more := z.TagAttr()
			attrs = more
			src := string(v)
			if string(k) == "src" && len(src) > 4 && strings.EqualFold(src[:4], "cid:") {
				ids = append(ids, src[4:])
				break
			}
		}
		if len(ids) > mailbudget.Parts {
			break
		}
	}
	return ids
}
func referenced(raw string, refs []Ref, original map[string]*parsemail.Attachment) ([]string, error) {
	explicit := map[string]bool{}
	if len(refs) > MaxRefs {
		return nil, ErrRef
	}
	for _, r := range refs {
		if len(r.CID) > 255 || r.CID == "" {
			return nil, ErrRef
		}
		explicit[r.CID] = true
	}
	var ids []string
	for _, cid := range sourceCIDs(raw) {
		if original[cid] != nil {
			delete(explicit, cid)
			continue
		}
		if !cidFormat.MatchString(cid) {
			return nil, ErrRef
		}
		if !slices.Contains(ids, cid) {
			ids = append(ids, cid)
		}
		delete(explicit, cid)
	}
	if len(ids) > MaxRefs || len(explicit) > 0 {
		return nil, ErrRef
	}
	return ids, nil
}
func activeTx(tx *xorm.Session, uid int, ids []string) error {
	now := time.Now()
	for _, cid := range ids {
		// Conditional touch serializes against expiry and carves access by owner.
		// A draft reference keeps an otherwise idle image active.
		result, err := tx.Exec("UPDATE compose_image SET touched=? WHERE user_id=? AND cid=? AND (touched>=? OR EXISTS (SELECT 1 FROM draft_inline_image WHERE draft_inline_image.cid=compose_image.cid))", now.Unix(), uid, cid, now.Add(-ImageLife).Unix())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			// MySQL may report zero for a same-second timestamp assignment.
			ok, err := tx.Where("user_id=? AND cid=? AND touched>=?", uid, cid, now.Unix()).Exist(&models.ComposeImage{})
			if err != nil {
				return err
			}
			if !ok {
				return ErrRef
			}
		}
	}
	return nil
}

// SyncDraft runs inside the owner-locked draft write. The relationship, not a
// duplicate blob, protects referenced images while a draft remains saved.
func SyncDraft(tx *xorm.Session, uid, id int, raw string, refs []Ref) error {
	ids, err := referenced(raw, refs, nil)
	if err != nil {
		return err
	}
	if err = activeTx(tx, uid, ids); err != nil {
		return err
	}
	if err = DeleteDraft(tx, id); err != nil {
		return err
	}
	for _, cid := range ids {
		if _, err = tx.Insert(&models.DraftInlineImage{DraftID: id, CID: cid}); err != nil {
			return err
		}
	}
	return nil
}
func DeleteDraft(tx *xorm.Session, id int) error {
	_, err := tx.Where("draft_id=?", id).Delete(&models.DraftInlineImage{})
	return err
}
func Apply(ctx context.Context, uid int, e *parsemail.Email, refs []Ref, source []*parsemail.Attachment) error {
	original := map[string]*parsemail.Attachment{}
	for _, a := range source {
		if a != nil && a.ContentID != "" && raster(a.Content) != "" {
			original[a.ContentID] = a
		}
	}
	ids, err := referenced(string(e.HTML), refs, original)
	if err != nil {
		return err
	}
	for _, cid := range sourceCIDs(string(e.HTML)) {
		a := original[cid]
		if a == nil {
			continue
		}
		copy := *a
		copy.Inline = true
		restored := false
		for i, old := range e.Attachments {
			if old.ContentID == cid {
				e.Attachments[i] = &copy
				restored = true
				break
			}
		}
		if !restored {
			e.Attachments = append(e.Attachments, &copy)
		}
	}
	if len(ids) > 0 {
		err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
			tx.Context(ctx)
			if err := activeTx(tx, uid, ids); err != nil {
				return err
			}
			n := 0
			for _, cid := range ids {
				var u models.ComposeImage
				ok, err := tx.Where("user_id=? AND cid=?", uid, cid).Get(&u)
				if err != nil {
					return err
				}
				if !ok {
					return ErrRef
				}
				n += u.Size
				if n > mailbudget.Attachments {
					return mailbudget.ErrLimit
				}
				e.Attachments = append(e.Attachments, &parsemail.Attachment{Filename: u.Filename, ContentType: u.ContentType, ContentID: u.CID, Content: u.Content, Inline: true})
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if len(e.Text) == 0 && len(e.HTML) > 0 {
		e.Text = []byte(parsemail.HTMLText(string(e.HTML)))
	}
	return nil
}
func Cleanup(ctx context.Context, now time.Time) error {
	_, err := db.Instance.Context(ctx).Exec("DELETE FROM draft_inline_image WHERE NOT EXISTS (SELECT 1 FROM email WHERE email.id=draft_inline_image.draft_id)")
	if err != nil {
		return err
	}
	_, err = db.Instance.Context(ctx).Exec("DELETE FROM compose_image WHERE touched<? AND NOT EXISTS (SELECT 1 FROM draft_inline_image WHERE draft_inline_image.cid=compose_image.cid)", now.Add(-ImageLife).Unix())
	return err
}
func Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for ctx.Err() == nil {
			_ = Cleanup(ctx, time.Now())
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return done
}
