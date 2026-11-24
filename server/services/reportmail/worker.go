package reportmail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/maintenance"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

const batchSize = 64
const Retention = 400 * 24 * time.Hour

var wake = make(chan struct{}, 1)

// Wake coalesces work; the database is the queue, so overflow, restarts, and
// concurrently committed SMTP transactions cannot lose reports.
func Wake() {
	select {
	case wake <- struct{}{}:
	default:
	}
}

func metadata(ctx context.Context, rows []models.Email) (map[int][]*parsemail.Attachment, error) {
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		if r.ReportMailState == "" {
			ids = append(ids, r.Id)
		}
	}
	out := make(map[int][]*parsemail.Attachment)
	if len(ids) == 0 {
		return out, nil
	}
	var parts []models.MailAttachmentInfo
	if err := db.Instance.Context(ctx).Cols("email_id", "filename", "content_type").In("email_id", ids).Asc("email_id", "part").Find(&parts); err != nil {
		return nil, err
	}
	for _, p := range parts {
		out[p.EmailID] = append(out[p.EmailID], &parsemail.Attachment{Filename: p.Filename, ContentType: p.ContentType})
	}
	return out, nil
}

func storedParts(ctx context.Context, id int) ([]*parsemail.Attachment, error) {
	var row models.Email
	ok, err := db.Instance.Context(ctx).ID(id).Cols("attachments").Get(&row)
	if err != nil || !ok || row.Attachments == "" {
		return nil, err
	}
	var parts []*parsemail.Attachment
	if json.Unmarshal([]byte(row.Attachments), &parts) != nil {
		return nil, errPayload
	}
	return parts, nil
}

// Coordinate with last-link deletion: persist the statistics even if the
// attachment has just been removed, but never retain a stale original ID.
func liveEmail(tx *xorm.Session, id int) (int, error) {
	if id == 0 {
		return 0, nil
	}
	q := tx.ID(id).Cols("id")
	if tx.Engine().DriverName() != config.DBTypeSQLite {
		q.ForUpdate()
	}
	ok, err := q.Get(&models.Email{})
	if !ok {
		return 0, err
	}
	return id, err
}

func storeDMARC(ctx context.Context, row aggregate) error {
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		r := row.Report // Insert populates ID; keep retries independent.
		owned, err := tx.Where("name=?", r.Domain).Exist(&models.Domain{})
		if err != nil || !owned {
			return err
		}
		r.EmailID, err = liveEmail(tx, r.EmailID)
		if err != nil {
			return err
		}
		if _, err := tx.Insert(&r); err != nil {
			return err
		}
		// Bound variables below SQLite's legacy limit as well.
		for i := 0; i < len(row.Records); i += 50 {
			children := row.Records[i:min(i+50, len(row.Records))]
			for j := range children {
				children[j].ReportID = r.ID
			}
			if _, err := tx.Insert(&children); err != nil {
				return err
			}
		}
		return nil
	})
	if db.IsUniqueViolation(err) {
		return nil
	}
	return err
}

func storeTLS(ctx context.Context, row models.TLSReport) error {
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		r := row
		owned, err := tx.Where("name=?", r.Domain).Exist(&models.Domain{})
		if err != nil || !owned {
			return err
		}
		r.EmailID, err = liveEmail(tx, r.EmailID)
		if err != nil {
			return err
		}
		_, err = tx.Insert(&r)
		return err
	})
	if db.IsUniqueViolation(err) {
		return nil
	}
	return err
}

func scan(ctx context.Context, now time.Time) (int, error) {
	var rows []models.Email
	cutoff := now.Add(-Retention)
	err := db.Instance.Context(ctx).Cols("id", "subject", "create_time", "report_mail_state", "projection_ready").
		Where("type=? AND create_time>=?", consts.EmailTypeReceive, cutoff).
		In("report_mail_state", "", string(DMARCAggregate), string(TLSReport)).Asc("create_time", "id").Limit(batchSize).Find(&rows)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	meta, err := metadata(ctx, rows)
	if err != nil {
		return 0, err
	}
	var first error
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return len(rows), err
		}
		if err := process(ctx, row, meta[row.Id]); err != nil && first == nil {
			first = err
		}
	}
	return len(rows), first
}

func process(ctx context.Context, row models.Email, meta []*parsemail.Attachment) error {
	kind, err := classifyStored(ctx, row, meta)
	if err != nil {
		return err
	}
	parseErr := false
	if kind == DMARCAggregate || kind == TLSReport {
		parts, err := storedParts(ctx, row.Id)
		if err != nil && !errors.Is(err, errPayload) {
			return err
		}
		parsed, err := parseParts(kind, parts)
		if err != nil {
			parseErr = true
		} else {
			for _, r := range parsed.DMARC {
				r.Report.EmailID, r.Report.ReceivedAt = row.Id, row.CreateTime.Unix()
				if err := storeDMARC(ctx, r); err != nil {
					return err
				}
			}
			for _, r := range parsed.TLS {
				r.EmailID, r.ReceivedAt = row.Id, row.CreateTime.Unix()
				if err := storeTLS(ctx, r); err != nil {
					return err
				}
			}
		}
	}
	n, err := db.Instance.Context(ctx).NoAutoTime().Table(&models.Email{}).ID(row.Id).Where("report_mail_state=?", row.ReportMailState).
		Update(map[string]any{"report_mail_state": ":" + string(kind)})
	if parseErr && err == nil && n > 0 {
		log.Warnf("mail report attachment rejected email_id=%d kind=%s", row.Id, kind)
	}
	return err
}

// Start runs the startup backfill and continues to consume persisted candidates.
// A tick covers worker/SMTP timing races and maintenance cancellation.
func Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTimer(0)
		defer t.Stop()
		for ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-wake:
			}
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			next := 30 * time.Second
			maintenance.Work(ctx, func(work context.Context) {
				n, err := scan(work, time.Now())
				if err != nil && work.Err() == nil {
					log.Warn("mail report worker could not process a batch; will retry")
				}
				if err == nil && n == batchSize {
					next = 25 * time.Millisecond
				}
			})
			t.Reset(next)
		}
	}()
	return done
}

func Purge(ctx context.Context, now time.Time) error {
	before := now.Add(-Retention).Unix()
	for _, t := range []string{"dmarc_report", "tls_report"} {
		for ctx.Err() == nil {
			var ids []int
			if err := db.Instance.Context(ctx).Table(t).Cols("id").Where("received_at<?", before).Asc("received_at", "id").Limit(batchSize).Find(&ids); err != nil {
				return err
			}
			if len(ids) == 0 {
				break
			}
			if err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
				tx.Context(ctx)
				if strings.HasPrefix(t, "dmarc") {
					if _, err := tx.In("report_id", ids).Delete(&models.DMARCRecord{}); err != nil {
						return err
					}
				}
				model := any(&models.DMARCReport{})
				if t == "tls_report" {
					model = &models.TLSReport{}
				}
				_, err := tx.In("id", ids).Where("received_at<?", before).Delete(model)
				return err
			}); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
