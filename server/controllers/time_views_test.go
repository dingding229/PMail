package controllers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/user"
)

func TestAPITimeViewsKeepLegacyTimesAndAddISO(t *testing.T) {
	instant := time.Date(2026, 11, 22, 23, 17, 0, 0, time.FixedZone("+08", 8*3600))
	const expected = `"2026-11-22T15:17:00Z"`
	for _, tc := range []struct {
		name       string
		view       any
		set, empty []string
	}{
		{"user", apiUser(models.User{Created: instant}), []string{"created"}, []string{"last_login_at"}},
		{"domain", apiDomain(models.Domain{Created: instant, Updated: instant}), []string{"created", "updated"}, nil},
		{"DNS", apiDNSGuide(domain.DNSGuide{CheckedAt: instant}), []string{"checked_at"}, nil},
		{"queue", apiQueueItem(models.OutboundQueue{Created: instant, Updated: instant, NextRetryAt: instant}), []string{"created", "updated", "next_retry_at"}, nil},
		{"token", apiToken(models.APIToken{Created: instant, ExpiresAt: &instant}), []string{"created", "expires_at"}, []string{"last_used_at"}},
		{"forward", apiForward(models.ForwardSetting{Updated: instant}), []string{"updated"}, nil},
		{"address", apiAddress(address.View{MailboxAddress: models.MailboxAddress{Created: instant}}), []string{"created"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.view)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			for _, field := range tc.set {
				if string(object[field]) != `"2026-11-22T23:17:00+08:00"` || string(object[field+"_iso"]) != expected {
					t.Fatalf("legacy or ISO time missing for %s: %s", field, data)
				}
			}
			for _, field := range tc.empty {
				if string(object[field]) != "null" || string(object[field+"_iso"]) != "null" {
					t.Fatalf("optional time must be null: %s", data)
				}
			}
		})
	}
	data, err := json.Marshal(apiCurrentUser(user.View{User: models.User{Created: instant, LastLoginAt: &instant}, Addresses: []address.View{{MailboxAddress: models.MailboxAddress{Created: instant}}}}))
	if err != nil {
		t.Fatal(err)
	}
	var compound struct {
		LastLoginISO json.RawMessage `json:"last_login_at_iso"`
		Addresses    []struct {
			CreatedISO json.RawMessage `json:"created_iso"`
		} `json:"addresses"`
	}
	if err := json.Unmarshal(data, &compound); err != nil || string(compound.LastLoginISO) != expected || len(compound.Addresses) != 1 || string(compound.Addresses[0].CreatedISO) != expected {
		t.Fatalf("compound user time view: %s (%v)", data, err)
	}
}
