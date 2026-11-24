package send

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/textproto"
	"os"
	"syscall"
	"testing"
	"time"
)

type dualStackFixture struct{}

func (dualStackFixture) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "mx.example.net.", Pref: 10}}, nil
}
func (dualStackFixture) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("2a00:1450:4025:c01::1a"), netip.MustParseAddr("142.250.110.26")}, nil
}

// A host without an IPv6 route must not report "network is unreachable"
// when the IPv4 server gave a real answer, and IPv4 is tried first.
func TestOutboundPrefersIPv4AndKeepsServerReply(t *testing.T) {
	var attempts []string
	policy := Outbound{Resolver: dualStackFixture{}, TLSRequired: map[string]bool{}, Dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
		attempts = append(attempts, address)
		if address[0] == '[' {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}
		}
		srv, client := net.Pipe()
		go func() {
			defer srv.Close()
			_ = srv.SetDeadline(time.Now().Add(3 * time.Second))
			_, _ = fmt.Fprint(srv, "421 4.7.0 Try again later\r\n")
		}()
		return client, nil
	}}
	err := policy.deliver(context.Background(), "example.net", "sender@example.test", "example.test", []string{"rcpt@example.net"}, []byte("x"))
	var reply *textproto.Error
	if !errors.As(err, &reply) || reply.Code != 421 {
		t.Fatalf("got %v, want the 421 reply", err)
	}
	if len(attempts) != 2 || attempts[0] != "142.250.110.26:25" {
		t.Fatalf("dial order %v, want IPv4 first", attempts)
	}
}

func TestMoreSpecificError(t *testing.T) {
	unreachable := &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}
	reply := &textproto.Error{Code: 450, Msg: "busy"}
	if got := moreSpecific(reply, unreachable); got != reply {
		t.Fatalf("unreachable replaced reply: %v", got)
	}
	if got := moreSpecific(unreachable, errors.New("timeout")); got.Error() != "timeout" {
		t.Fatalf("connection error should replace unreachable: %v", got)
	}
}
