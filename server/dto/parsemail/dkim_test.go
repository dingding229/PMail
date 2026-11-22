package parsemail_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/Jinnrry/pmail/services/inboundauth"
	"github.com/emersion/go-msgauth/dkim"
)

func TestInboundDKIMDoesNotTreatMissingSignatureAsPass(t *testing.T) {
	engine := inboundauth.Engine{VerifyDKIM: func(_ context.Context, raw []byte, _ inboundauth.Resolver) ([]*dkim.Verification, error) {
		if string(raw) != "From: invalid\r\n\r\nfixture" {
			t.Fatal("verifier did not receive the original message")
		}
		return nil, nil
	}}
	got := engine.Verify(context.Background(), netip.Addr{}, "", "", []byte("From: invalid\r\n\r\nfixture"))
	if got.DKIM.Result != "none" || got.DKIM.Aligned || got.Status() == "pass" {
		t.Fatalf("unsigned result: %+v", got)
	}
}
