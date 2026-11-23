package inbound

import (
	"errors"
	"testing"

	"github.com/Jinnrry/pmail/models"
)

func TestClamAVSettingsAndEnvironmentOverride(t *testing.T) {
	e, _, _ := inboundFixture(t)
	ctx := t.Context()
	t.Setenv("PMAIL_CLAMAV", "")
	check := func(enabled, control bool) {
		t.Helper()
		e, c, err := ClamAVPolicy(ctx)
		if err != nil || e != enabled || c != control {
			t.Fatalf("enabled=%v env=%v err=%v", e, c, err)
		}
	}
	check(false, false)
	if _, err := e.Insert(&models.InboundSettings{ID: 1, ZonesJSON: "[]", DefaultSpamThreshold: 9, ClamdEnabled: true, ClamdAddr: "deprecated:3310"}); err != nil {
		t.Fatal(err)
	}
	check(false, false) // an old external endpoint is never auto-enabled
	if err := SetClamAV(ctx, true); err != nil {
		t.Fatal(err)
	}
	check(true, false)
	t.Setenv("PMAIL_CLAMAV", "false")
	check(false, true)
	if err := SetClamAV(ctx, false); !errors.Is(err, ErrClamAVEnv) {
		t.Fatalf("overridden write: %v", err)
	}
	t.Setenv("PMAIL_CLAMAV", "")
	if _, err := PutSettings(ctx, Settings{Zones: []string{}, DefaultSpamThreshold: 8}, CatchAllUpdate{}); err != nil {
		t.Fatal(err)
	}
	check(true, false)
	if err := SetClamAV(ctx, false); err != nil {
		t.Fatal(err)
	}
	check(false, false)
	t.Setenv("PMAIL_CLAMAV", "TRUE")
	check(true, true)
	t.Setenv("PMAIL_CLAMAV", "not-a-bool")
	check(false, false)
}
