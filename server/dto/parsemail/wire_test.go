package parsemail

import (
	"errors"
	"net/textproto"
	"testing"
)

func TestC7MailBuildRejectsControlCharactersWithoutExit(t *testing.T) {
	base := Email{From: &User{EmailAddress: "from@example.net"}, To: []*User{{EmailAddress: "to@example.net"}}, Subject: "normal"}
	if raw, err := base.BuildBytes(nil, false); err != nil || len(raw) == 0 {
		t.Fatalf("valid message: %v", err)
	}
	for _, modify := range []func(*Email){
		func(e *Email) { e.Subject = "broken\r\nline" },
		func(e *Email) { e.Headers = textproto.MIMEHeader{"X-Fixture": {"invalid\x00"}} },
		func(e *Email) { e.Headers = textproto.MIMEHeader{"From": {"other@example.net"}} },
		func(e *Email) { e.To = []*User{{EmailAddress: "invalid\nto@example.net"}} },
		func(e *Email) { e.Attachments = []*Attachment{{Filename: "fixture\r", ContentType: "text/plain"}} },
	} {
		v := base
		modify(&v)
		if _, err := v.BuildBytes(nil, false); !errors.Is(err, ErrInvalidMailHeader) {
			t.Fatalf("invalid header accepted: %v", err)
		}
	}
}
