package reportmail

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

func fixture(t *testing.T) *xorm.Engine {
	t.Helper()
	e, err := db.Open(&config.Config{DbType: config.DBTypeSQLite, DbDSN: "file:report-fixture?mode=memory&cache=shared&_pragma=foreign_keys(ON)"}, false)
	if err != nil {
		t.Fatal(err)
	}
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = old; _ = e.Close() })
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.Domain{Name: "example.test", IsDefault: true}); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestReportBackfillDedupRetentionAndDomainIsolation(t *testing.T) {
	e := fixture(t)
	ctx := t.Context()
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	add := func(subject, state, doc string, age time.Duration) int {
		t.Helper()
		r := models.Email{Subject: subject, ReportMailState: state, CreateTime: now.Add(-age), Attachments: jsonText([]*parsemail.Attachment{{Filename: "sample.xml", ContentType: "text/xml", Content: []byte(doc)}})}
		if _, err := e.NoAutoTime().Insert(&r); err != nil {
			t.Fatal(err)
		}
		return r.Id
	}
	first := add("Report Domain: example.test Submitter: example.test", "", google, time.Hour)
	add("Duplicate", string(DMARCAggregate), google, time.Hour)
	add("New foreign report", string(DMARCAggregate), strings.ReplaceAll(google, "example.test", "unknown.test"), time.Hour)
	add("Older than retention", string(DMARCAggregate), yahoo, Retention+time.Hour)
	bad := add("Broken", string(DMARCAggregate), "<feedback>", time.Hour)
	if n, err := scan(ctx, now); err != nil || n != 4 {
		t.Fatalf("first scan=%d %v", n, err)
	}
	if n, err := scan(ctx, now); err != nil || n != 0 {
		t.Fatalf("repeat scan=%d %v", n, err)
	}
	var wanted models.DMARCReport
	if n, err := e.Count(&wanted); err != nil || n != 1 {
		t.Fatalf("reports %d %v", n, err)
	}
	if ok, err := e.Get(&wanted); !ok || err != nil || wanted.EmailID != first {
		t.Fatalf("duplicate or out of domain: %+v %v", wanted, err)
	}
	if n, err := e.Where("report_id=?", wanted.ID).Count(&models.DMARCRecord{}); err != nil || n != 1 {
		t.Fatalf("records %d %v", n, err)
	}
	var row models.Email
	if ok, err := e.ID(bad).Get(&row); !ok || err != nil || row.ReportMailState != ":"+string(DMARCAggregate) {
		t.Fatalf("invalid attachment should be marked done: %+v %v", row, err)
	}
	// SQLite enforces the new physical FK when enabled in the connection.
	if _, err := e.Insert(&models.DMARCRecord{ReportID: wanted.ID + 999, Count: 1}); err == nil {
		t.Fatal("orphan DMARC child accepted")
	}
	rowTLS := models.TLSReport{Domain: "example.test", OrgName: "sender", ReportID: "tls-expired", ReceivedAt: now.Add(-Retention - time.Second).Unix(), FailureDetails: "[]"}
	for j := 0; j < 2; j++ {
		if err := storeTLS(ctx, rowTLS); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := e.Count(&models.TLSReport{}); err != nil || n != 1 {
		t.Fatalf("TLS reports %d %v", n, err)
	}
	if err := Purge(ctx, now.Add(Retention)); err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&models.DMARCReport{}, &models.DMARCRecord{}, &models.TLSReport{}} {
		if n, err := e.Count(m); err != nil || n != 0 {
			t.Fatalf("purge %T %d %v", m, n, err)
		}
	}
}

func TestReportStatisticsWindowAndVisibility(t *testing.T) {
	e := fixture(t)
	ctx := t.Context()
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	w, err := ResolveWindow(ctx, "example.test", "2026-11-23", "2026-11-24", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.Email{Id: 101}); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []string{google, microsoft, yahoo, strings.ReplaceAll(strings.ReplaceAll(yahoo, "1795392000", "1600000000"), "1795478399", "1600086399")} {
		r, err := parseDMARC([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		r.Report.EmailID = 101
		if err := storeDMARC(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.Insert(&models.UserEmail{EmailID: 101, UserID: 5, GroupId: 200}); err != nil {
		t.Fatal(err)
	}
	rows, err := parseTLS([]byte(tlsJSONFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if err := storeTLS(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Summary(ctx, w, func(_ context.Context, name string, ips []netip.Addr) map[netip.Addr]bool {
		if name != "example.test" || len(ips) != 3 {
			t.Fatalf("lookup %s %v", name, ips)
		}
		return map[netip.Addr]bool{netip.MustParseAddr("192.0.2.4"): true}
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.ReportCount != 3 || s.Totals.Total != 21 || s.Totals.DMARCPass != 15 || s.Totals.DMARCPassRate != 71.43 || s.Totals.SPFPass != 12 || s.Totals.DKIMPass != 3 || s.Totals.Quarantined != 12 || s.Totals.Rejected != 6 {
		t.Fatalf("summary %+v", s)
	}
	if len(s.Daily) != 2 || s.Daily[0] != (Day{"2026-11-23", 15, 6}) || s.Daily[1] != (Day{"2026-11-24", 0, 0}) || len(s.Reporters) != 3 || s.Reporters[0].Count != 12 || s.Reporters[0].PassRate != 100 {
		t.Fatal(s.Daily, s.Reporters)
	}
	if len(s.Sources) != 3 || !s.Sources[0].IsOwn || s.Sources[0].Suspicious || !s.Sources[1].Suspicious || s.Sources[1].IsOwn || s.Sources[2].Suspicious {
		t.Fatal(s.Sources)
	}
	if s.TLS == nil || s.TLS.SuccessCount != 27 || s.TLS.FailureCount != 2 || len(s.TLS.FailureTypes) != 1 || s.TLS.FailureTypes[0].Count != 2 || s.LatestPolicy == nil {
		t.Fatal(s.TLS, s.LatestPolicy)
	}
	p, err := Records(ctx, w, 5, 1, "fail")
	if err != nil || p.Total != 1 || len(p.Items) != 1 || p.Items[0].SPFDomain != "relay.example.test" || p.Items[0].SPFAuth != "pass" || p.Items[0].EmailID != 101 {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = Records(ctx, w, 6, 1, "fail")
	if err != nil || len(p.Items) != 1 || p.Items[0].EmailID != 0 {
		t.Fatalf("not owned: %+v %v", p, err)
	}
	l, err := Reports(ctx, w, 5, 1)
	if err != nil || l.Total != 4 || len(l.Items) != 4 || l.Items[0].BeginAt.Format(time.DateOnly) != "2026-11-23" {
		t.Fatalf("reports %+v %v", l, err)
	}
	// TB25: list rows must carry their primary keys, not 0.
	for _, it := range l.Items {
		if it.ID <= 0 {
			t.Fatalf("report without id: %+v", it)
		}
	}
	if p, err := Records(ctx, w, 5, 1, "all"); err != nil || len(p.Items) == 0 || p.Items[0].ID <= 0 {
		t.Fatalf("record without id: %+v %v", p, err)
	}
	if _, err := ResolveWindow(ctx, "not-ours.test", "", "", now); err != ErrQuery {
		t.Fatal(err)
	}
}

func TestApplyMetadataAndFolderRecreation(t *testing.T) {
	e := fixture(t)
	ctx := t.Context()
	u := models.User{Account: "reportowner"}
	if _, err := e.Insert(&u); err != nil {
		t.Fatal(err)
	}
	for i, report := range []bool{true, false, true} {
		r := models.Email{Subject: fmt.Sprintf("mail %d", i), ProjectionReady: true}
		if _, err := e.Insert(&r); err != nil {
			t.Fatal(err)
		}
		if report {
			if _, err := e.Insert(&models.MailAttachmentInfo{EmailID: r.Id, Filename: "provider.test!example.test!1795392000!1795478399.zip", ContentType: "application/zip"}); err != nil {
				t.Fatal(err)
			}
		}
		status := int8(0)
		if i == 2 {
			status = 5
		}
		if _, err := e.Insert(&models.UserEmail{UserID: u.ID, EmailID: r.Id, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Apply(ctx, u.ID)
	if err != nil || r.Moved != 1 || r.GroupID == 0 {
		t.Fatalf("apply %+v %v", r, err)
	}
	old := r.GroupID
	if _, err := e.Table(&models.UserEmail{}).Where("group_id=?", old).Update(map[string]any{"group_id": 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ID(old).Delete(&models.Group{}); err != nil {
		t.Fatal(err)
	}
	r, err = Apply(ctx, u.ID)
	if err != nil || r.Moved != 1 || r.GroupID == old {
		t.Fatalf("recreation %+v %v", r, err)
	}
}

func TestBackgroundWorkerConsumesPersistedTLS(t *testing.T) {
	e := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	parts := []*parsemail.Attachment{{Filename: "rpt.json.gz", ContentType: "application/tlsrpt+gzip", Content: gzipped(t, tlsJSONFixture)}}
	row := models.Email{Subject: "classified from wire headers", ReportMailState: InitialState(TLSReport), Attachments: jsonText(parts)}
	if _, err := e.Insert(&row); err != nil {
		t.Fatal(err)
	}
	worker := Start(ctx)
	t.Cleanup(func() { cancel(); <-worker })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var saved models.TLSReport
		ok, err := e.Get(&saved)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if saved.EmailID != row.Id || saved.SuccessCount != 27 || saved.FailureCount != 2 {
				t.Fatal(saved)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("startup worker did not consume persisted TLS report")
}
