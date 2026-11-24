package mailaddress

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

var ErrForbidden = errors.New("mail address does not belong to user")
var ErrInvalid = errors.New("invalid mailbox address filter")

func engine() *xorm.Engine { return db.Instance }

type Filter struct {
	ID    int
	Other bool
}

// Validate is deliberately scoped even when the caller is an administrator.
// JSON clients may send either the numeric ID or its decimal representation.
func Validate(owner int, raw string) (Filter, error) {
	switch raw {
	case "", "all":
		return Filter{}, nil
	case "other":
		return Filter{Other: true}, nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 || strconv.Itoa(id) != raw {
		return Filter{}, ErrInvalid
	}
	ok, err := engine().ID(id).Where("user_id=? AND status=?", owner, models.AddressActive).Exist(&models.MailboxAddress{})
	if err != nil {
		return Filter{}, err
	}
	if !ok {
		return Filter{}, ErrForbidden
	}
	return Filter{ID: id}, nil
}

func Value(query string, body json.RawMessage) (string, error) {
	if query != "" {
		return query, nil
	}
	if len(body) == 0 || string(body) == "null" {
		return "", nil
	}
	if body[0] == '"' {
		var s string
		if err := json.Unmarshal(body, &s); err != nil {
			return "", ErrInvalid
		}
		return s, nil
	}
	return string(body), nil
}

// Predicate operates on ue (user_email). Using EXISTS keeps a single row per
// membership even if a message was delivered to multiple aliases.
const otherPredicate = `ue.catch_all_addresses IS NOT NULL AND ue.catch_all_addresses <> '' AND ue.catch_all_addresses <> '[]'`

func (f Filter) Predicate() (string, []any) {
	if f.Other {
		return otherPredicate, nil
	}
	if f.ID <= 0 {
		return "", nil
	}
	return `EXISTS (SELECT 1 FROM user_email_address x JOIN mailbox_address a ON a.id=x.address_id WHERE x.user_email_id=ue.id AND x.address_id=? AND a.user_id=ue.user_id AND a.status='active')`, []any{f.ID}
}

type Choice struct {
	Address `xorm:"extends"`
	Unread  int `json:"unread"`
}

type Counts struct {
	Addresses []Choice `json:"addresses"`
	AllUnread int      `json:"all_unread"`
	Other     struct {
		Unread  int  `json:"unread"`
		HasMail bool `json:"has_mail"`
	} `json:"other"`
}

func Unread(owner int) (Counts, error) {
	result := Counts{Addresses: []Choice{}}
	var rows []row
	if err := engine().SQL(`SELECT `+addressColumns+fromAddress+`WHERE a.user_id=? AND a.status='active' ORDER BY a.is_primary DESC,a.id`, owner).Find(&rows); err != nil {
		return result, err
	}
	type unreadRow struct {
		AddressID int `xorm:"address_id"`
		Unread    int
	}
	var counts []unreadRow
	// Align with Web inbox semantics including IMAP's explicit inbox group.
	const inbox = ` ue.is_read=0 AND ue.status=0 AND (ue.group_id=0 OR ue.group_id=?) AND e.type=0`
	const ownerMail = ` FROM user_email ue JOIN email e ON e.id=ue.email_id `
	if err := engine().SQL(`SELECT x.address_id,COUNT(DISTINCT ue.email_id) AS unread`+ownerMail+`JOIN user_email_address x ON x.user_email_id=ue.id WHERE ue.user_id=? AND`+inbox+` GROUP BY x.address_id`, owner, models.INBOX).Find(&counts); err != nil {
		return result, err
	}
	byID := map[int]int{}
	for _, c := range counts {
		byID[c.AddressID] = c.Unread
	}
	for _, r := range rows {
		result.Addresses = append(result.Addresses, Choice{Address: r.view(), Unread: byID[r.ID]})
	}
	for i, dest := range []*int{&result.AllUnread, &result.Other.Unread} {
		q := `SELECT COUNT(DISTINCT ue.email_id)` + ownerMail + `WHERE ue.user_id=? AND` + inbox
		if i == 1 {
			q += ` AND (` + otherPredicate + `)`
		}
		if _, err := engine().SQL(q, owner, models.INBOX).Get(dest); err != nil {
			return result, err
		}
	}
	var found int
	ok, err := engine().SQL(`SELECT 1 FROM user_email ue WHERE ue.user_id=? AND (`+strings.TrimSpace(otherPredicate)+`) LIMIT 1`, owner).Get(&found)
	result.Other.HasMail = ok
	return result, err
}
