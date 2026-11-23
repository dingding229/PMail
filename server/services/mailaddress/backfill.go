package mailaddress

import (
	"context"
	"slices"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

const BatchSize = 128

type historicLink struct {
	ID                  int `xorm:"id"`
	UserID              int `xorm:"user_id"`
	Type, MailStatus    int
	FromAddress, To, Cc string
	CatchAllAddresses   string
}

// Backfill is a restartable background migration. The membership marker is
// written in the same transaction as the links, including known non-matches.
// Current deliveries never go through header-based inference on restart.
func Backfill(ctx context.Context, e *xorm.Engine) (int, error) {
	var progress models.UserEmailAddressProgress
	if _, err := e.Context(ctx).ID(1).Get(&progress); err != nil {
		return 0, err
	}
	processed, last := 0, progress.ThroughID
	for ctx.Err() == nil {
		var batch []historicLink
		var wrote, through int
		err := db.Transaction(e, func(tx *xorm.Session) error {
			batch, wrote, through = nil, 0, last
			tx.Context(ctx)
			var window []int
			if err := tx.Table(&models.UserEmail{}).Where("id>?", last).Asc("id").Limit(BatchSize).Cols("id").Find(&window); err != nil {
				return err
			}
			if len(window) == 0 {
				return nil
			}
			through = window[len(window)-1]
			query := historyQuery(e) + ` WHERE ue.id>? AND ue.id<=? AND ue.address_attributed=? ORDER BY ue.id`
			if err := tx.SQL(query, last, through, false).Find(&batch); err != nil {
				return err
			}

			owners := make([]int, 0, len(batch))
			for _, r := range batch {
				owners = append(owners, r.UserID)
			}
			slices.Sort(owners)
			c, err := loadCatalog(tx, slices.Compact(owners))
			if err != nil {
				return err
			}
			for _, r := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				n, err := tx.ID(r.ID).Where("address_attributed=?", false).Cols("address_attributed").Update(&models.UserEmail{AddressAttributed: true})
				if err != nil {
					return err
				}
				if n == 0 {
					continue
				} // a concurrent edit already attributed or removed this link
				if err := insert(tx, models.UserEmail{ID: r.ID, UserID: r.UserID}, c.match(r.UserID, r.targets())); err != nil {
					return err
				}
				wrote++
			}
			var checkpoint models.UserEmailAddressProgress
			found, err := tx.ID(1).Get(&checkpoint)
			if err != nil {
				return err
			}
			if !found {
				_, err = tx.Insert(&models.UserEmailAddressProgress{ID: 1, ThroughID: through})
			} else if checkpoint.ThroughID < through {
				_, err = tx.ID(1).Where("through_id<?", through).Cols("through_id").Update(&models.UserEmailAddressProgress{ThroughID: through})
			}
			return err
		})
		if err != nil {
			return processed, err
		}
		processed += wrote
		if through == last {
			return processed, nil
		}
		last = through
		// Yield the single SQLite writer connection and avoid sustained load on servers.
		select {
		case <-ctx.Done():
			return processed, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	return processed, ctx.Err()
}

func historyQuery(e *xorm.Engine) string {
	return `SELECT ue.id,ue.user_id,e.type,e.status AS mail_status,e.from_address,e.` + e.Quote("to") + `,e.cc,ue.catch_all_addresses
		FROM user_email ue JOIN email e ON e.id=ue.email_id`
}

func (r historicLink) targets() []string {
	switch {
	case r.Type == int(consts.EmailTypeSend), r.MailStatus == int(consts.EmailStatusDrafts):
		return []string{r.FromAddress}
	case len(CatchAll(r.CatchAllAddresses)) == 0:
		return append(HeaderTargets(r.To), HeaderTargets(r.Cc)...)
	default:
		return nil
	}
}
