package token

import (
	"testing"
	"time"

	"github.com/Jinnrry/pmail/models"
)

func TestC7TokenScopeAndExpiryPolicy(t *testing.T) {
	if Scope(models.APIToken{}) != Legacy || !Allows(Legacy, "POST", "/api/token/create") {
		t.Fatal("legacy permissions were not preserved")
	}
	if !Allows(Read, "GET", "/api/attachments/preview/1.0") || !Allows(Read, "POST", "/api/email/list") || Allows(Read, "POST", "/api/email/send") || Allows(Read, "POST", "/api/user/delete") {
		t.Fatal("read scope mismatch")
	}
	if !Allows(Send, "POST", "/api/email/send") || Allows(Send, "POST", "/api/domain/create") || Allows(Admin, "POST", "/api/token/create") {
		t.Fatal("send/admin scope mismatch")
	}
	now := time.Now()
	date, err := expiry(now, nil, false)
	if err != nil || date.Sub(now) != 90*24*time.Hour {
		t.Fatal("default expiry mismatch")
	}
	if date, err := expiry(now, nil, true); err != nil || date != nil {
		t.Fatal("explicit no-expiry rejected")
	}
	if _, err := expiry(now, &now, true); err == nil {
		t.Fatal("conflicting expiry accepted")
	}
}
