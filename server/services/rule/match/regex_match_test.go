package match

import (
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

func TestRuleRegexRejectsLegacyLookaroundAndOversize(t *testing.T) {
	for _, s := range []string{"*", `^(?!.*abc\.com).*`, strings.Repeat("x", MaxPatternBytes+1)} {
		if _, err := Compile(s); err == nil {
			t.Fatalf("invalid regex accepted: %q", s)
		}
		if NewRegexMatch("Subject", s).Match(nil, &parsemail.Email{Subject: "fixture"}) {
			t.Fatal("invalid legacy rule matched")
		}
	}
	if !NewRegexMatch("Subject", `^mail:?`).Match(nil, &parsemail.Email{Subject: "mail fixture"}) {
		t.Fatal("valid regex did not match")
	}
}
