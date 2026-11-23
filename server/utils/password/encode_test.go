package password

import "testing"

func TestLegacyVerificationRequiresUpgrade(t *testing.T) {
	stored := legacyHash("fixture-password")
	if valid, upgrade := Verify(stored, "fixture-password"); !valid || !upgrade {
		t.Fatal("legacy value must require migration")
	}
	if valid, upgrade := Verify(stored, "wrong"); valid || upgrade {
		t.Fatal("invalid password accepted")
	}
}
