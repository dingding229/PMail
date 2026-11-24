package email

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestChangesWriteDeadlineAndBadParameters(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = old; e.Close() })
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	ctx := &context.Context{Context: t.Context(), UserID: 1}
	for _, q := range []string{"?wait=26", "?wait=-1", "?wait=", "?wait=1.0", "?cursor=bad", "?cursor=ok.2.-1", "?cursor=ok.18446744073709551616.1", "?cursor=ok.2.9223372036854775808", "?cursor=ok.00.1"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, changesPath+q, nil)
		Changes(ctx, w, r)
		if w.Code != http.StatusBadRequest || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("bad query %q accepted (%d)", q, w.Code)
		}
	}
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	before := time.Now()
	Changes(ctx, w, httptest.NewRequest(http.MethodGet, changesPath+"?wait=2", nil))
	if w.Code != 200 || w.deadline.Before(before.Add(12*time.Second)) || w.deadline.After(time.Now().Add(12*time.Second)) {
		t.Fatalf("wrong request deadline: %v status=%d", w.deadline, w.Code)
	}
}
