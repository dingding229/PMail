package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

func ensurePayload(tx *xorm.Session, raw []byte) (int, error) {
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	if raw == nil {
		raw = []byte{}
	}
	sql := "INSERT INTO outbound_payload (digest, wire_data) VALUES (?,?) ON CONFLICT (digest) DO UPDATE SET digest=EXCLUDED.digest"
	if db.Instance.DriverName() == config.DBTypeMySQL {
		sql = "INSERT INTO outbound_payload (digest, wire_data) VALUES (?,?) ON DUPLICATE KEY UPDATE digest=VALUES(digest)"
	}
	// Write or lock the existing digest before inserting its new reference.
	if _, err := tx.Exec(sql, digest, raw); err != nil {
		return 0, err
	}
	var row models.OutboundPayload
	ok, err := tx.Where("digest=?", digest).Cols("id").Get(&row)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.New("outbound payload missing after insert")
	}
	return row.ID, nil
}

// Upgrade legacy copies progressively while workers keep reading them. A
// leased row is skipped; the next pass catches it when the lease ends.
func migrateLegacyPayloads(ctx context.Context) {
	for ctx.Err() == nil {
		if err := migratePayloadPass(ctx); err != nil && ctx.Err() == nil {
			log.Errorf("queue payload migration: %v", err)
		}
		wait(ctx, time.Hour)
	}
}

func migratePayloadPass(ctx context.Context) error {
	cursor := 0
	for ctx.Err() == nil {
		var keys []models.OutboundQueue
		if err := db.Instance.Context(ctx).Where("payload_id=0 AND id>? AND status<>?", cursor, StatusSending).Cols("id").Asc("id").Limit(32).Find(&keys); err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		for _, key := range keys {
			cursor = key.ID
			if err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
				var row models.OutboundQueue
				ok, err := tx.Context(ctx).ID(key.ID).Where("payload_id=0 AND status<>?", StatusSending).Cols("raw_data").Get(&row)
				if err != nil || !ok {
					return err
				}
				id, err := ensurePayload(tx, row.RawData)
				if err != nil {
					return err
				}
				n, err := tx.Table(&models.OutboundQueue{}).ID(key.ID).Where("payload_id=0 AND status<>?", StatusSending).NoAutoTime().Update(map[string]any{"payload_id": id, "raw_data": []byte{0}})
				if err != nil {
					return err
				}
				if n == 0 {
					return errPayloadRace
				}
				return nil
			}); err != nil && !errors.Is(err, errPayloadRace) {
				return err
			}
		}
		wait(ctx, 10*time.Millisecond)
	}
	return ctx.Err()
}

var errPayloadRace = errors.New("outbound queue changed during payload migration")
