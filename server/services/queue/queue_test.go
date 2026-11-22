package queue

import (
	"errors"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func newQueueTestEngine(t *testing.T) *xorm.Engine {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1) // SQLite :memory: databases are private to each connection.
	if err := testschema.Sync(engine); err != nil {
		engine.Close()
		t.Fatal(err)
	}
	old := db.Instance
	db.Instance = engine
	t.Cleanup(func() { db.Instance = old; _ = engine.Close() })
	return engine
}

func TestQueueSQLiteLifecycleAndParentStatus(t *testing.T) {
	engine := newQueueTestEngine(t)
	user := &models.User{Account: "sender", Name: "sender"}
	if _, err := engine.Insert(user); err != nil {
		t.Fatal(err)
	}
	email := &models.Email{FromAddress: "sender@example.com", Status: 6}
	if _, err := engine.Insert(email); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.UserEmail{UserID: user.ID, EmailID: email.Id, Status: 6}); err != nil {
		t.Fatal(err)
	}
	row, err := Enqueue(KindSend, user.ID, email.Id, "sender@example.com", "remote@example.net", []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := claimOne()
	if err != nil || !ok || claimed.ID != row.ID || claimed.Attempts != 1 {
		t.Fatalf("claim = %+v, ok=%v, err=%v", claimed, ok, err)
	}
	handleSuccess(claimed)
	var saved models.OutboundQueue
	if ok, err := engine.ID(row.ID).Get(&saved); err != nil || !ok || saved.Status != StatusSent {
		t.Fatalf("queue status = %+v, ok=%v, err=%v", saved, ok, err)
	}
	var relation models.UserEmail
	if ok, err := engine.Where("email_id=? AND user_id=?", email.Id, user.ID).Get(&relation); err != nil || !ok || relation.Status != 1 {
		t.Fatalf("parent relation = %+v, ok=%v, err=%v", relation, ok, err)
	}
}

func TestQueueSQLiteRetryMaxAttemptsAndLeaseRecovery(t *testing.T) {
	engine := newQueueTestEngine(t)
	row, err := Enqueue(KindSend, 1, 0, "sender@example.com", "remote@example.net", []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ID(row.ID).Cols("max_attempts", "status", "lease_until").Update(&models.OutboundQueue{MaxAttempts: 1, Status: StatusSending, LeaseUntil: ptrTime(time.Now().Add(-time.Minute))}); err != nil {
		t.Fatal(err)
	}
	if err := recoverExpiredLeases(time.Now()); err != nil {
		t.Fatal(err)
	}
	var reset models.OutboundQueue
	if ok, err := engine.ID(row.ID).Get(&reset); err != nil || !ok || reset.Status != StatusPending || reset.LeaseUntil != nil {
		t.Fatalf("lease recovery = %+v, ok=%v, err=%v", reset, ok, err)
	}
	claimed, ok, err := claimOne()
	if err != nil || !ok {
		t.Fatalf("claim after recovery: %+v %v %v", claimed, ok, err)
	}
	handleFailure(contextForQueueTest{}, claimed, errors.New("permanent test failure"))
	var failed models.OutboundQueue
	if ok, err := engine.ID(row.ID).Get(&failed); err != nil || !ok || failed.Status != StatusFailed || failed.Attempts != 1 {
		t.Fatalf("failed queue = %+v, ok=%v, err=%v", failed, ok, err)
	}
}

type contextForQueueTest struct{}

func (contextForQueueTest) Deadline() (time.Time, bool) { return time.Time{}, false }
func (contextForQueueTest) Done() <-chan struct{}       { return nil }
func (contextForQueueTest) Err() error                  { return nil }
func (contextForQueueTest) Value(any) any               { return nil }

func ptrTime(value time.Time) *time.Time { return &value }

func TestReconcileParentHandlesEitherCompletionOrder(t *testing.T) {
	engine := newQueueTestEngine(t)
	user := &models.User{Account: "parent", Name: "parent"}
	if _, err := engine.Insert(user); err != nil {
		t.Fatal(err)
	}
	email := &models.Email{FromAddress: "parent@example.com", Status: 6}
	if _, err := engine.Insert(email); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.UserEmail{UserID: user.ID, EmailID: email.Id, Status: 6}); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileParent(email.Id, user.ID); err != nil {
		t.Fatal(err)
	}
	child, err := Enqueue(KindSend, user.ID, email.Id, "parent@example.com", "remote@example.net", []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Instance.ID(child.ID).Cols("status").Update(&models.OutboundQueue{Status: StatusSent}); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileParent(email.Id, user.ID); err != nil {
		t.Fatal(err)
	}
	var relation models.UserEmail
	if ok, err := engine.Where("email_id=? AND user_id=?", email.Id, user.ID).Get(&relation); err != nil || !ok || relation.Status != 1 {
		t.Fatalf("completed parent = %+v, ok=%v, err=%v", relation, ok, err)
	}
}
