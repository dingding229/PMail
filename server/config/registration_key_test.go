package config

import (
	"os"
	"regexp"
	"testing"
)

func TestRegistrationKeyPersistsWithRestrictedPermissions(t *testing.T) {
	prev := ROOT_PATH
	ROOT_PATH = t.TempDir()
	t.Cleanup(func() { ROOT_PATH = prev })
	key, err := EnsureRegistrationKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(registrationAlphabet) != 32 || !regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}(-[A-HJ-NP-Z2-9]{4}){3}$`).MatchString(key) {
		t.Fatal("registration key does not have four groups from the 32-symbol alphabet")
	}
	info, err := os.Stat(RegistrationKeyPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
	if again, err := EnsureRegistrationKey(); err != nil || key != again {
		t.Fatalf("registration key changed across startup: %v", err)
	}
}
