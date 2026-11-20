package domain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/utils/dkimkey"
)

type DNSRecord struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Expected    string   `json:"expected"`
	Actual      []string `json:"actual"`
	Match       bool     `json:"match"`
	LookupError string   `json:"lookup_error,omitempty"`
}

func DNS(ctx context.Context, id int) ([]DNSRecord, error) {
	row, ok := GetByID(id)
	if !ok {
		return nil, ErrNotFound
	}
	d, key, ok := Signer(row.Name)
	if !ok {
		return nil, ErrNotFound
	}
	pub, err := dkimkey.Record(key)
	if err != nil {
		return nil, err
	}
	out := []DNSRecord{
		{Type: "MX", Name: d.Name, Expected: "10 smtp." + d.Name + "."},
		{Type: "TXT", Name: d.Name, Expected: "v=spf1 a mx ~all"},
		{Type: "TXT", Name: d.DkimSelector + "._domainkey." + d.Name, Expected: pub},
		{Type: "TXT", Name: "_dmarc." + d.Name, Expected: "v=DMARC1; p=none"},
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for i := range out {
		r := &out[i]
		r.Actual = []string{}
		var err error
		if r.Type == "MX" {
			mx, lookupErr := net.DefaultResolver.LookupMX(ctx, r.Name+".")
			err = lookupErr
			for _, m := range mx {
				r.Actual = append(r.Actual, fmt.Sprintf("%d %s", m.Pref, m.Host))
			}
		} else {
			actual, lookupErr := net.DefaultResolver.LookupTXT(ctx, r.Name+".")
			err = lookupErr
			r.Actual = append(r.Actual, actual...)
		}
		for _, v := range r.Actual {
			normalize := func(s string) string { return strings.Join(strings.Fields(strings.ReplaceAll(s, "; ", ";")), " ") }
			if normalize(v) == normalize(r.Expected) || (r.Type == "MX" && strings.EqualFold(v, r.Expected)) {
				r.Match = true
			}
		}
		var dnsErr *net.DNSError
		if err != nil && !(errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
			r.LookupError = err.Error()
		}
	}
	return out, nil
}
