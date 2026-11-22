package pop3_server

import (
	"net"
	"testing"
	"time"
)

func TestC7bPOP3AdmissionAndIdlePolicy(t *testing.T) {
	var b connectionBudget
	address := socketSource(&net.TCPAddr{IP: net.ParseIP("::ffff:192.0.2.10"), Port: 995})
	if address != "192.0.2.10" {
		t.Fatal(address)
	}
	a, ok := b.reserve(address, 2, 1)
	if !ok {
		t.Fatal("first was blocked")
	}
	if _, ok = b.reserve(address, 2, 1); ok {
		t.Fatal("per-IP limit ignored")
	}
	other, ok := b.reserve("192.0.2.20", 2, 1)
	if !ok {
		t.Fatal("second source was blocked")
	}
	if _, ok = b.reserve("192.0.2.30", 2, 1); ok {
		t.Fatal("global limit ignored")
	}
	a()
	other()
	if b.total != 0 || len(b.sources) != 0 {
		t.Fatal("slots not released")
	}
	t.Setenv("PMAIL_POP3_IDLE_SECONDS", "7")
	if popIdleTimeout() != 7*time.Second {
		t.Fatal("idle setting ignored")
	}
}
