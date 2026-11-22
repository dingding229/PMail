package setup

import (
	"fmt"
	"strings"

	"github.com/Jinnrry/pmail/config"

	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
	"github.com/Jinnrry/pmail/utils/ip"
)

type DNSItem struct {
	Type  string `json:"type"`
	Host  string `json:"host"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
	Tips  string `json:"tips"`
}

func GetDNSSettings(ctx *context.Context) (map[string][]*DNSItem, error) {
	configData, err := config.ReadEffectiveConfig()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	record, err := auth.EnsureDKIM(configData.DkimPrivateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	ret := make(map[string][]*DNSItem)

	// A web hostname belongs to its most specific configured DNS zone, not
	// to every mail domain. An external web zone is configured separately.
	webZone := ""
	webHost := strings.TrimSuffix(strings.ToLower(configData.WebDomain), ".")
	for _, domain := range configData.Domains {
		zone := strings.ToLower(domain)
		if len(domain) > len(webZone) && (webHost == zone || strings.HasSuffix(webHost, "."+zone)) {
			webZone = domain
		}
	}
	for _, domain := range configData.Domains {
		ret[domain] = []*DNSItem{
			{Type: "A", Host: "smtp", Value: ip.GetIp(), TTL: 3600, Tips: i18n.GetText(ctx.Lang, "ip_taps")},
			{Type: "A", Host: "imap", Value: ip.GetIp(), TTL: 3600, Tips: i18n.GetText(ctx.Lang, "ip_taps")},
			{Type: "A", Host: "pop", Value: ip.GetIp(), TTL: 3600, Tips: i18n.GetText(ctx.Lang, "ip_taps")},
			{Type: "A", Host: "@", Value: ip.GetIp(), TTL: 3600, Tips: i18n.GetText(ctx.Lang, "ip_taps")},
			{Type: "MX", Host: "@", Value: fmt.Sprintf("smtp.%s", domain), TTL: 3600},
			{Type: "TXT", Host: "@", Value: "v=spf1 a mx ~all", TTL: 3600},
			{Type: "TXT", Host: "default._domainkey", Value: record, TTL: 3600},
		}
		if domain == webZone {
			host := strings.TrimSuffix(webHost, "."+strings.ToLower(domain))
			switch host {
			case strings.ToLower(domain), "smtp", "imap", "pop":
				// These A records already appear in the list.
			default:
				ret[domain] = append(ret[domain], &DNSItem{
					Type: "A", Host: host, Value: ip.GetIp(), TTL: 3600, Tips: i18n.GetText(ctx.Lang, "ip_taps"),
				})
			}
		}
	}

	return ret, nil
}
