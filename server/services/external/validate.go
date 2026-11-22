package external

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/domain"
)

var ErrHostRequired = errors.New("external host is required")
var ErrPrivateHost = errors.New("external host resolves to a private or local address")

func validateHost(host string) error {
	if strings.TrimSpace(host) == "" {
		return ErrHostRequired
	}
	if allowPrivate() {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "localhost" {
		return ErrPrivateHost
	}
	for _, local := range append([]string{config.Instance.Domain, config.Instance.WebDomain}, domain.Names()...) {
		if strings.EqualFold(lower, strings.TrimSpace(local)) {
			return ErrPrivateHost
		}
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			return err
		}
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return ErrPrivateHost
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if allowPrivate() {
		return false
	}
	if mapped := ip.To4(); mapped != nil {
		ip = mapped
		first := mapped[0]
		if first == 0 || (first == 100 && mapped[1] >= 64 && mapped[1] <= 127) {
			return true
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func safeAddresses(host string, port int) ([]string, error) {
	if err := validateHost(host); err != nil {
		return nil, err
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			return nil, err
		}
	}
	addresses := make([]string, 0, len(ips))
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, ErrPrivateHost
		}
		addresses = append(addresses, net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	}
	if len(addresses) == 0 {
		return nil, ErrPrivateHost
	}
	return addresses, nil
}

func allowPrivate() bool { return strings.EqualFold(os.Getenv("PMAIL_FETCH_ALLOW_PRIVATE"), "true") }
