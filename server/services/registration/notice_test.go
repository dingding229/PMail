package registration

import (
	"bytes"
	"strings"
	"testing"
)

func TestC7RegistrationNoticeDoesNotLogKeyByDefault(t *testing.T) {
	var out bytes.Buffer
	const sample = "FAKE-REGISTRATION-SECRET"
	writeNotice(&out, sample, "https://mail.example.test/", false)
	if strings.Contains(out.String(), sample) || !strings.Contains(out.String(), "registration_key") {
		t.Fatal("default notice omitted path or included secret")
	}
	out.Reset()
	writeNotice(&out, sample, "https://mail.example.test/", true)
	if !strings.Contains(out.String(), sample) {
		t.Fatal("explicit compatibility notice omitted key")
	}
}
