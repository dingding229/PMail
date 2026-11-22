package smtp

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestC7bPinnedGreetingRespectsCallerDeadline(t *testing.T) {
	srv, client := net.Pipe()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- SendPinned(ctx, client, "mx.example.test", "from@example.test", "example.test", []string{"to@example.net"}, nil, VerifySTARTTLS)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("SMTP greeted on silent socket")
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP greeting did not respect deadline")
	}
}
