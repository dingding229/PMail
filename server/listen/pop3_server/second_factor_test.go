package pop3_server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/testschema/mailauth"
)

func TestPOP3SecondFactorCredentialMatrix(t *testing.T) {
	user := popFixture(t)
	mailauth.MailCredentialMatrix(t, user, func(email, secret string) bool {
		s, c := net.Pipe()
		done := make(chan struct{})
		go func() { defer close(done); defer s.Close(); serveConnection(s, "mail-credentials.test", true, nil) }()
		defer func() { _ = c.Close(); <-done }()
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		r := bufio.NewReader(c)
		read := func() string {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			return line
		}
		command := func(line string) string {
			if _, err := fmt.Fprintf(c, "%s\r\n", line); err != nil {
				t.Fatal(err)
			}
			return read()
		}
		if !strings.HasPrefix(read(), "+OK") || !strings.HasPrefix(command("USER "+email), "+OK") {
			t.Fatal("missing POP3 greeting or USER acknowledgement")
		}
		result := command("PASS " + secret)
		if strings.HasPrefix(result, "+OK") {
			if !strings.HasPrefix(command("STAT"), "+OK") {
				t.Fatal("invalid POP3 session")
			}
			return true
		}
		if !strings.HasPrefix(result, "-ERR authentication failed") {
			t.Fatalf("unexpected PASS response: %q", result)
		}
		return false
	})
}
