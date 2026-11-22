package external

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/domain"
)

var ErrHostRequired = errors.New("external host is required")
var ErrPrivateHost = errors.New("external host resolves to a private or local address")

func validateHost(host string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := safeAddresses(ctx, host, 0)
	return err
}

func isBlockedIP(ip net.IP) bool {
	if allowPrivate() {
		return false
	}
	if mapped := ip.To4(); mapped != nil {
		ip = mapped
		if mapped[0] == 0 || (mapped[0] == 100 && mapped[1] >= 64 && mapped[1] <= 127) {
			return true
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func safeAddresses(ctx context.Context, host string, port int) ([]string, error) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return nil, ErrHostRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !allowPrivate() {
		if host == "localhost" {
			return nil, ErrPrivateHost
		}
		for _, local := range append([]string{config.Instance.Domain, config.Instance.WebDomain}, domain.Names()...) {
			if strings.EqualFold(host, strings.TrimSuffix(strings.TrimSpace(local), ".")) {
				return nil, ErrPrivateHost
			}
		}
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		values, err := net.DefaultResolver.LookupIPAddr(ctx, host+".")
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			ips = append(ips, value.IP)
		}
	}
	var out []string
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, ErrPrivateHost
		}
		out = append(out, net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	}
	if len(out) == 0 {
		return nil, ErrPrivateHost
	}
	return out, nil
}

func allowPrivate() bool { return strings.EqualFold(os.Getenv("PMAIL_FETCH_ALLOW_PRIVATE"), "true") }
