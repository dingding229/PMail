package smtp_server

import (
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestC7bAnonymousRelayAndCatchAllMatrix(t *testing.T) {
	e := newAuthenticationTestEngine(t)
	oldDB, oldCfg := db.Instance, config.Instance
	db.Instance = e
	config.Instance = &config.Config{Domain: "example.test", Domains: []string{"example.test"}, CatchAll: true, SecretKey: "fixture"}
	t.Cleanup(func() { db.Instance = oldDB; config.Instance = oldCfg })
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	owner := models.User{Account: "owner", Password: "fixture"}
	admin := models.User{Account: "admin", Password: "fixture", IsAdmin: 1}
	if _, err := e.Insert(&owner, &admin); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	const known = "owner@example.test"
	const unknown = "unknown@example.test"
	const remote = "person@example.net"
	for _, catchAll := range []bool{false, true} {
		config.Instance.CatchAll = catchAll
		s := &Session{Ctx: &context.Context{}}
		for _, tc := range []struct {
			to string
			ok bool
		}{{known, true}, {unknown, catchAll}, {remote, false}} {
			if err := s.Rcpt(tc.to, nil); (err == nil) != tc.ok {
				t.Fatalf("%t RCPT %s: %v", catchAll, tc.to, err)
			}
		}
		users, drop, err := resolveIncomingUsers(s.Ctx, &parsemail.Email{}, 0, []string{known, unknown, remote}, true, true)
		if err != nil || drop || len(users) != 1+intBool(catchAll) || users[0].ID != owner.ID {
			t.Fatalf("mixed delivery: %v drop=%t %+v", err, drop, users)
		}
		if users, drop, err := resolveIncomingUsers(s.Ctx, &parsemail.Email{}, 0, []string{remote}, true, true); err != nil || !drop || len(users) != 0 {
			t.Fatal("non-local caught by catch-all")
		}
	}
	bounce, err := forward.BuildSRS("someone@example.net", "example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = queue.Enqueue(queue.KindForward, admin.ID, 0, bounce, remote, []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	s := &Session{Ctx: &context.Context{}}
	if err = s.Rcpt(bounce, nil); err != nil {
		t.Fatalf("valid SRS recipient rejected: %v", err)
	}
}
func intBool(b bool) int {
	if b {
		return 1
	}
	return 0
}
