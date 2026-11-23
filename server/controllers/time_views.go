package controllers

import (
	"time"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/token"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/utils/isotime"
)

// Keep timestamp additions explicit at the API boundary. The underlying
// models retain their original JSON and database behavior.
func mapViews[T, U any](rows []T, toView func(T) U) []U {
	if rows == nil {
		return nil
	}
	ret := make([]U, len(rows))
	for i, row := range rows {
		ret[i] = toView(row)
	}
	return ret
}

type userISOTimes struct {
	CreatedISO     *time.Time `json:"created_iso"`
	LastLoginAtISO *time.Time `json:"last_login_at_iso"`
}

func userTimes(row models.User) userISOTimes {
	return userISOTimes{isotime.UTC(row.Created), isotime.Optional(row.LastLoginAt)}
}
func apiUser(row models.User) any {
	return struct {
		models.User
		userISOTimes
	}{row, userTimes(row)}
}

type apiAddressView struct {
	address.View
	CreatedISO *time.Time `json:"created_iso"`
}

func apiAddress(row address.View) apiAddressView {
	return apiAddressView{row, isotime.UTC(row.Created)}
}
func apiCurrentUser(row user.View) any {
	return struct {
		user.View
		userISOTimes
		Addresses []apiAddressView `json:"addresses"`
	}{row, userTimes(row.User), mapViews(row.Addresses, apiAddress)}
}

func apiDomain(row models.Domain) any {
	return struct {
		models.Domain
		CreatedISO *time.Time `json:"created_iso"`
		UpdatedISO *time.Time `json:"updated_iso"`
	}{row, isotime.UTC(row.Created), isotime.UTC(row.Updated)}
}

func apiDNSGuide(row domain.DNSGuide) any {
	return struct {
		domain.DNSGuide
		CheckedAtISO *time.Time `json:"checked_at_iso"`
	}{row, isotime.UTC(row.CheckedAt)}
}

func apiQueueItem(row models.OutboundQueue) any {
	return struct {
		models.OutboundQueue
		NextRetryAtISO *time.Time `json:"next_retry_at_iso"`
		CreatedISO     *time.Time `json:"created_iso"`
		UpdatedISO     *time.Time `json:"updated_iso"`
	}{row, isotime.UTC(row.NextRetryAt), isotime.UTC(row.Created), isotime.UTC(row.Updated)}
}

func apiToken(row models.APIToken) any {
	row.Scope = token.Scope(row)
	return struct {
		models.APIToken
		LastUsedAtISO *time.Time `json:"last_used_at_iso"`
		ExpiresAtISO  *time.Time `json:"expires_at_iso"`
		CreatedISO    *time.Time `json:"created_iso"`
		LegacyScope   bool       `json:"legacy_scope"`
		NeverExpires  bool       `json:"never_expires"`
	}{row, isotime.Optional(row.LastUsedAt), isotime.Optional(row.ExpiresAt), isotime.UTC(row.Created), row.Scope == token.Legacy, row.ExpiresAt == nil}
}

func apiForward(row models.ForwardSetting) any {
	return struct {
		models.ForwardSetting
		UpdatedISO *time.Time `json:"updated_iso"`
	}{row, isotime.UTC(row.Updated)}
}
