// Package safenet decides which addresses server-side fetches may reach, so
// user-supplied URLs cannot be used to probe the server's own network.
package safenet

import (
	"context"
	"errors"
	"net"
	"net/netip"
)

var denied = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), // NAT64 can map to private IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"), // 6to4
	netip.MustParsePrefix("2001::/32"), // Teredo
}

// PublicAddress reports whether ip is a global unicast address outside every
// private, local and tunnelling range.
func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, p := range denied {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

var ErrBlocked = errors.New("address not allowed")

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type DialFunc func(context.Context, string, string) (net.Conn, error)

// PublicDial resolves the host once, rejects the whole answer if any address
// is not public, and dials only the checked numeric address so the transport
// never re-resolves (no DNS rebinding).
func PublicDial(resolve Resolver, dial DialFunc) DialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		var ips []netip.Addr
		if literal, e := netip.ParseAddr(host); e == nil {
			ips = []netip.Addr{literal}
		} else if ips, err = resolve.LookupNetIP(ctx, "ip", host); err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, ErrBlocked
		}
		for _, ip := range ips {
			if !PublicAddress(ip) {
				return nil, ErrBlocked
			}
		}
		for _, ip := range ips {
			c, e := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if e == nil {
				return c, nil
			}
			err = e
			if ctx.Err() != nil {
				break
			}
		}
		return nil, err
	}
}
