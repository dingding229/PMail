// Package mailaddress records and queries the address ownership of a mail copy.
package mailaddress

import (
	"encoding/json"
	"net/mail"
	"slices"
	"strings"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"xorm.io/builder"
	"xorm.io/xorm"
)

type Address struct {
	ID        int    `xorm:"id" json:"id"`
	Address   string `json:"address"`
	CanSend   bool   `json:"can_send"`
	IsPrimary bool   `json:"is_primary"`
}

type row struct {
	Address    `xorm:"extends"`
	UserID     int `xorm:"user_id"`
	LinkID     int `xorm:"link_id"`
	LocalPart  string
	DomainName string
}

const addressColumns = `a.id,a.user_id,a.local_part,a.can_send,a.is_primary,d.name AS domain_name`
const fromAddress = ` FROM mailbox_address a JOIN domain d ON d.id=a.domain_id `

func (r row) view() Address {
	r.Address.Address = r.LocalPart + "@" + r.DomainName
	return r.Address
}

type catalog map[int]map[string]int

func loadCatalog(tx *xorm.Session, owners []int) (catalog, error) {
	c := catalog{}
	if len(owners) == 0 {
		return c, nil
	}
	var rows []row
	condition, args, err := builder.Select(addressColumns).From("mailbox_address a").Join("INNER", "domain d", "d.id=a.domain_id").Where(builder.In("a.user_id", owners)).ToSQL()
	if err != nil {
		return nil, err
	}
	if err := tx.SQL(condition, args...).Find(&rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if c[r.UserID] == nil {
			c[r.UserID] = map[string]int{}
		}
		c[r.UserID][r.LocalPart+"@"+r.DomainName] = r.ID
	}
	return c, nil
}

func (c catalog) match(owner int, targets []string) []int {
	var ids []int
	for _, target := range targets {
		local, host, err := address.Split(target)
		if err != nil {
			continue
		}
		if id := c[owner][local+"@"+host]; id > 0 {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// Add writes the real matching addresses for already-inserted copies in their
// delivery transaction. An unmatched recipient never acquires an address ID.
func Add(tx *xorm.Session, links []models.UserEmail, targets []string) error {
	if len(targets) == 0 || len(links) == 0 {
		return nil
	}
	owners := make([]int, 0, len(links))
	for _, link := range links {
		owners = append(owners, link.UserID)
	}
	slices.Sort(owners)
	c, err := loadCatalog(tx, slices.Compact(owners))
	if err != nil {
		return err
	}
	for _, link := range links {
		actual := targets
		if skipped := CatchAll(link.CatchAllAddresses.String); len(skipped) > 0 {
			actual = slices.DeleteFunc(slices.Clone(targets), func(raw string) bool {
				l, h, err := address.Split(raw)
				return err == nil && slices.Contains(skipped, l+"@"+h)
			})
		}
		if err := insert(tx, link, c.match(link.UserID, actual)); err != nil {
			return err
		}
	}
	return nil
}

func insert(tx *xorm.Session, link models.UserEmail, ids []int) error {
	var rows []models.UserEmailAddress
	for _, id := range ids {
		var existing models.MailboxAddress
		// Lock only matched aliases. Alias deletion locks/deletes the address
		// before removing mappings, so in-flight delivery cannot leave stale links.
		query := `SELECT id FROM mailbox_address WHERE id=? AND user_id=?`
		switch tx.Engine().DriverName() {
		case "postgres":
			query += " FOR KEY SHARE"
		case "mysql":
			query += " LOCK IN SHARE MODE"
		}
		ok, err := tx.SQL(query, id, link.UserID).Get(&existing)
		if err != nil {
			return err
		}
		if ok {
			rows = append(rows, models.UserEmailAddress{UserEmailID: link.ID, AddressID: id})
		}
	}
	if len(rows) > 0 {
		_, err := tx.Insert(&rows)
		return err
	}
	return nil
}

func ReplaceDraft(tx *xorm.Session, owner, emailID int, from string) error {
	var links []models.UserEmail
	if err := tx.Where("user_id=? AND email_id=?", owner, emailID).Cols("id", "user_id").Find(&links); err != nil {
		return err
	}
	var ids []int
	for _, link := range links {
		ids = append(ids, link.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.In("id", ids).Cols("address_attributed").Update(&models.UserEmail{AddressAttributed: true}); err != nil {
		return err
	}
	if err := DeleteLinks(tx, ids); err != nil {
		return err
	}
	return Add(tx, links, []string{from})
}

func Copy(tx *xorm.Session, source models.UserEmail, dest models.UserEmail) error {
	if !source.AddressAttributed {
		// Do not publish new unprocessed IDs behind the migration checkpoint.
		// A pre-migration IMAP copy uses the same historical inference as backfill.
		var old historicLink
		ok, err := tx.SQL(historyQuery(tx.Engine())+` WHERE ue.id=?`, source.ID).Get(&old)
		if err != nil || !ok {
			return err
		}
		return Add(tx, []models.UserEmail{dest}, old.targets())
	}
	var links []models.UserEmailAddress
	if err := tx.Where("user_email_id=?", source.ID).Asc("address_id").Find(&links); err != nil {
		return err
	}
	ids := make([]int, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.AddressID)
	}
	return insert(tx, dest, ids)
}

func DeleteLinks(tx *xorm.Session, ids []int) error {
	for len(ids) > 0 {
		n := min(250, len(ids))
		if _, err := tx.In("user_email_id", ids[:n]).Delete(&models.UserEmailAddress{}); err != nil {
			return err
		}
		ids = ids[n:]
	}
	return nil
}

func Views(owner int, ids []int) (map[int][]Address, error) {
	result := map[int][]Address{}
	if len(ids) == 0 {
		return result, nil
	}
	var rows []row
	q := builder.Select(addressColumns+`, x.user_email_id AS link_id`).From("user_email_address x").Join("INNER", "mailbox_address a", "a.id=x.address_id").Join("INNER", "domain d", "d.id=a.domain_id").Join("INNER", "user_email ue", "ue.id=x.user_email_id").Where(builder.Eq{"a.user_id": owner, "ue.user_id": owner}.And(builder.In("x.user_email_id", ids))).OrderBy("a.id")
	sql, args, err := q.ToSQL()
	if err != nil {
		return nil, err
	}
	if err := engine().SQL(sql, args...).Find(&rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.LinkID] = append(result[r.LinkID], r.view())
	}
	return result, nil
}

// ForMessage returns the caller's matched addresses, never Bcc or another
// owner's address registry. IMAP folder copies may share the same matches.
func ForMessage(owner, email int) ([]Address, error) {
	var rows []row
	q := `SELECT DISTINCT ` + addressColumns + fromAddress + ` JOIN user_email_address x ON x.address_id=a.id JOIN user_email ue ON ue.id=x.user_email_id WHERE a.user_id=? AND ue.user_id=? AND ue.email_id=? ORDER BY a.is_primary DESC,a.id`
	if err := engine().SQL(q, owner, owner, email).Find(&rows); err != nil {
		return nil, err
	}
	out := make([]Address, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view())
	}
	return out, nil
}

func CatchAll(raw string) []string {
	var out []string
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func HeaderTargets(value string) []string {
	var raw []struct {
		EmailAddress string
		Address      string
	}
	if json.Unmarshal([]byte(value), &raw) == nil {
		result := make([]string, 0, len(raw))
		for _, entry := range raw {
			if entry.EmailAddress != "" {
				result = append(result, entry.EmailAddress)
			} else if entry.Address != "" {
				result = append(result, entry.Address)
			}
		}
		return result
	}
	return parseLegacyHeader(strings.TrimSpace(value))
}

func parseLegacyHeader(value string) []string {
	if value == "" {
		return nil
	}
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(parsed))
	for _, addr := range parsed {
		out = append(out, addr.Address)
	}
	return out
}
