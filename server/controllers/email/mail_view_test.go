package email

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/models"
)

func TestMailDetailPreservesUnzonedFieldsAndReturnsISOTimes(t *testing.T) {
	instant := time.Date(2026, 11, 22, 23, 17, 0, 0, time.FixedZone("+08", 8*3600))
	row := models.Email{SendDate: instant, CreateTime: instant, UpdateTime: instant, CronSendTime: instant}
	view, err := enrichedDetail(&row, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"send_date", "create_time", "update_time", "send_time"} {
		if string(view[name]) != `"2026-11-22 23:17:00"` || string(view[name+"_iso"]) != `"2026-11-22T15:17:00Z"` {
			t.Errorf("%s: legacy=%s iso=%s", name, view[name], view[name+"_iso"])
		}
	}
	if string(view["cron_send_time"]) != `"2026-11-22T23:17:00+08:00"` || string(view["cron_send_time_iso"]) != `"2026-11-22T15:17:00Z"` {
		t.Fatalf("cron send time: %s, %s", view["cron_send_time"], view["cron_send_time_iso"])
	}
	row.SendDate = time.Time{}
	view, err = enrichedDetail(&row, false, false, 0)
	if err != nil || string(view["send_date_iso"]) != "null" {
		t.Fatalf("zero timestamp: %s (%v)", view["send_date_iso"], err)
	}
}

func TestMailAuthenticationForDraftSentReceivedAndUnchecked(t *testing.T) {
	for _, tc := range []struct {
		name          string
		row           models.Email
		status        int8
		draft, danger bool
		auth          string
	}{
		{"new draft", models.Email{Status: consts.EmailStatusDrafts}, 0, true, false, "not_applicable"},
		{"old draft link", models.Email{}, consts.EmailStatusDrafts, true, false, "not_applicable"},
		{"sent", models.Email{Type: consts.EmailTypeSend}, 0, false, false, "not_applicable"},
		{"sent link", models.Email{}, consts.EmailStatusSent, false, false, "not_applicable"},
		{"inbound failed", models.Email{}, 0, false, true, "fail"},
		{"inbound passed", models.Email{DKIMCheck: 1}, 0, false, false, "pass"},
		{"external unchecked", models.Email{AuthUnchecked: true}, 0, false, false, "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := mailAuthentication(tc.row, tc.status)
			if auth.IsDraft != tc.draft || auth.Dangerous != tc.danger || auth.AuthStatus != tc.auth {
				t.Fatalf("mail authentication %+v", auth)
			}
			view, err := enrichedDetail(&tc.row, false, false, tc.status)
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := json.Marshal(tc.auth)
			if string(view["auth_status"]) != string(expected) || string(view["spf_check"]) != strconv.Itoa(int(tc.row.SPFCheck)) {
				t.Fatalf("detail missing auth status or legacy values: %v", view)
			}
		})
	}
}
