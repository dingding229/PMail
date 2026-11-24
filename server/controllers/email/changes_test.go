package email

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailevents"
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

func TestChangesRejectsStaleCredentialsBeforeReturningMail(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	oldDB, oldEvents := db.Instance, mailevents.Default
	db.Instance, mailevents.Default = e, mailevents.New()
	t.Cleanup(func() { db.Instance, mailevents.Default = oldDB, oldEvents; e.Close() })
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	u := models.User{Account: "changes-test", AuthVersion: 2, FactorVersion: 3}
	m := models.Email{Subject: "only visible with current credentials"}
	if _, err := e.Insert(&u, &m); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.UserEmail{UserID: u.ID, EmailID: m.Id}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		auth, factor int64
		disabled     int
		want         int
	}{
		{"current", 2, 3, 0, http.StatusOK},
		{"password reset", 1, 3, 0, http.StatusUnauthorized},
		{"2FA reset", 2, 2, 0, http.StatusUnauthorized},
		{"disabled", 2, 3, 1, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.Table(&models.User{}).ID(u.ID).Update(map[string]any{"disabled": tc.disabled}); err != nil {
				t.Fatal(err)
			}
			// Keep the version equal: wait briefly, then take the timeout path.
			r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s?cursor=%s.0.0&wait=1", changesPath, mailevents.Default.Epoch()), nil)
			w := httptest.NewRecorder()
			ctx := &context.Context{Context: t.Context(), UserID: u.ID, AuthVersion: tc.auth, FactorVersion: tc.factor}
			Changes(ctx, w, r)
			if w.Code != tc.want || strings.Contains(w.Body.String(), m.Subject) != (tc.want == http.StatusOK) {
				t.Fatalf("status=%d, expected %d; mail visibility inconsistent", w.Code, tc.want)
			}
		})
	}
}
