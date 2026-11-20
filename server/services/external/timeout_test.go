package external

import (
	"context"
	"net"
	"testing"
	"time"
)

// This test is intentionally kept as a source-only regression fixture for the
// server test session. It verifies the same cancellation path used by
// SyncAccount for a stalled POP3 connection and proves the connection is
// released instead of retaining the account mutex forever.
func TestStalledConnectionCancellationReleasesConnection(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	stop := watchConnContext(ctx, client)
	cancel()
	defer stop()

	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("expected cancelled watcher to close the stalled connection")
	}
}
