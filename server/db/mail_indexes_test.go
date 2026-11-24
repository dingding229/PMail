package db

import (
	"slices"
	"testing"

	"xorm.io/xorm"
)

func TestMailIndexesReplaceLegacyPrefixesWithoutDroppingData(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	e.SetMaxOpenConns(1)
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE INDEX "IDX_user_email_user_id" ON user_email (user_id)`,
		`CREATE INDEX "IDX_mailbox_address_status" ON mailbox_address (status)`,
		`CREATE INDEX "IDX_email_translation_email_id" ON email_translation (email_id)`,
		`CREATE INDEX operator_ue_group ON user_email (group_id)`,
		`INSERT INTO user_email (id,user_id,email_id,deleted_at,status) VALUES (7,11,13,123,3)`,
	} {
		if _, err := e.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := SyncSchema(e); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string][]string{
		"idx_ue_unread":                  {"user_id", "status", "is_read", "group_id"},
		"idx_ue_trash_expiry":            {"status", "user_id", "deleted_at", "id"},
		"idx_mailbox_address_review":     {"status", "requested_at", "id"},
		"operator_ue_group":              {"group_id"},
		`IDX_user_email_'idx_eid'`:       {"user_id", "email_id"},
		"IDX_email_ai_result_email_id":   {"email_id"},
		"IDX_user_email_user_id":         nil,
		"IDX_mailbox_address_status":     nil,
		"IDX_email_translation_email_id": nil,
	} {
		var cols []string
		if err := e.SQL(`SELECT name FROM pragma_index_info(?) ORDER BY seqno`, name).Find(&cols); err != nil || !slices.Equal(cols, want) {
			t.Errorf("index %s = %v, want %v, err=%v", name, cols, want, err)
		}
	}
	var ts int64
	if _, err := e.SQL(`SELECT deleted_at FROM user_email WHERE id=7 AND user_id=11 AND email_id=13`).Get(&ts); err != nil || ts != 123 {
		t.Fatalf("migration changed existing mail link: %d, %v", ts, err)
	}
}
