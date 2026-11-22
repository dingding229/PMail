package quota

import (
	"context"
	"errors"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestQuotaIncrementRollbackAndReconcile(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	defer e.Close()
	old := db.Instance
	db.Instance = e
	defer func() { db.Instance = old }()
	if err = testschema.Sync(e); err != nil {
		t.Fatal(err)
	}
	user := models.User{Account: "quota", Name: "Quota", Password: "fixture", QuotaMB: 1}
	if _, err = e.Insert(&user); err != nil {
		t.Fatal(err)
	}
	mail := models.Email{Size: 400000}
	if _, err = e.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Insert(&models.UserEmail{UserID: user.ID, EmailID: mail.Id}); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("fixture rollback")
	mutate := func(fail bool) error {
		return db.Transaction(e, func(tx *xorm.Session) error {
			if err := LockAndCheck(tx, user.ID, int64(mail.Size)); err != nil {
				return err
			}
			if _, err := tx.Insert(&models.UserEmail{UserID: user.ID, EmailID: mail.Id}); err != nil {
				return err
			}
			if err := Adjust(tx, user.ID, int64(mail.Size)); err != nil {
				return err
			}
			if fail {
				return abort
			}
			return nil
		})
	}
	verify := func(want int64) {
		t.Helper()
		used, err := Usage([]int{user.ID})
		if err != nil || used[user.ID] != want {
			t.Fatalf("quota=%v want %d err=%v", used, want, err)
		}
	}
	verify(400000)
	if err = mutate(true); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	verify(400000)
	if err = mutate(false); err != nil {
		t.Fatal(err)
	}
	verify(800000)
	if err = mutate(false); !errors.Is(err, ErrExceeded) {
		t.Fatalf("quota admission: %v", err)
	}
	if _, err = e.Table(&models.User{}).ID(user.ID).Update(map[string]any{"used_bytes": 5}); err != nil {
		t.Fatal(err)
	}
	verify(5)
	if err = Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	verify(800000)
}
