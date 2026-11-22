package pop3_server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestC7POP3RequiresSTLSBeforeAuthentication(t *testing.T) {
	for _, allow := range []string{"false", "true"} {
		t.Run(allow, func(t *testing.T) {
			t.Setenv("PMAIL_POP3_ALLOW_PLAINTEXT_AUTH", allow)
			s, c := net.Pipe()
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(3 * time.Second))
			go func() { defer s.Close(); serveConnection(s, "mail.example.test", false, nil) }()
			r := bufio.NewReader(c)
			read := func() string {
				line, err := r.ReadString('\n')
				if err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(line)
			}
			send := func(line string) {
				if _, err := fmt.Fprint(c, line+"\r\n"); err != nil {
					t.Fatal(err)
				}
			}
			_ = read()
			send("CAPA")
			_ = read()
			advertised := false
			for {
				line := read()
				if line == "." {
					break
				}
				advertised = advertised || line == "USER"
			}
			if advertised != (allow == "true") {
				t.Fatal("incorrect authorization capabilities")
			}
			send("USER fixture")
			reply := read()
			if allow == "true" && reply != "+OK" {
				t.Fatal(reply)
			}
			if allow == "false" {
				if !strings.Contains(reply, "STLS") {
					t.Fatal(reply)
				}
				send("PASS fixture")
				if !strings.Contains(read(), "STLS") {
					t.Fatal("PASS not refused before STLS")
				}
			}
		})
	}
}
