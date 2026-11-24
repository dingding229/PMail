package reportmail

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

const google = `<?xml version="1.0" encoding="UTF-8"?>
<feedback><version>1.0</version><report_metadata><org_name>google.test</org_name><email>synthetic@example.test</email><extra_contact_info>https://example.test/</extra_contact_info><report_id>2556873338071705901</report_id><date_range><begin>1795392000</begin><end>1795478399</end></date_range></report_metadata>
<policy_published><domain>example.test</domain><adkim>r</adkim><aspf>s</aspf><p>quarantine</p><sp>reject</sp><pct>75</pct><np>none</np></policy_published>
<record><row><source_ip>192.0.2.4</source_ip><count>12</count><policy_evaluated><disposition>quarantine</disposition><dkim>fail</dkim><spf>pass</spf><reason><type>local_policy</type></reason></policy_evaluated></row><identifiers><header_from>example.test</header_from></identifiers><auth_results><dkim><domain>dkim.example.test</domain><result>pass</result><selector>selector1</selector></dkim><dkim><domain>other.example.test</domain><result>fail</result></dkim><spf><domain>example.test</domain><result>pass</result></spf></auth_results></record></feedback>`

const microsoft = `<?xml version="1.0"?>
<feedback xmlns="http://dmarc.org/dmarc-xml/0.1"><report_metadata><org_name>Enterprise Outlook Test</org_name><email>synthetic@example.test</email><report_id>&lt;report-ms@example.test&gt;</report_id><date_range><begin>1795392000</begin><end>1795478400</end></date_range></report_metadata>
<policy_published><domain>example.test</domain><adkim>s</adkim><aspf>s</aspf><p>reject</p><sp>reject</sp><pct>100</pct><fo>1</fo></policy_published>
<record><row><source_ip>2001:db8::2</source_ip><count>6</count><policy_evaluated><disposition>reject</disposition><dkim>fail</dkim><spf>fail</spf></policy_evaluated></row><identifiers><envelope_to>synthetic.example.test</envelope_to><envelope_from>relay.example.test</envelope_from><header_from>example.test</header_from></identifiers><auth_results><spf><domain>relay.example.test</domain><scope>mfrom</scope><result>pass</result></spf></auth_results></record></feedback>`

const yahoo = `<?xml version="1.0"?>
<feedback><report_metadata><org_name>Yahoo Fixture</org_name><email>synthetic@example.test</email><report_id>1795392000.example.test</report_id><date_range><begin>1795392000</begin><end>1795478399</end></date_range></report_metadata>
<policy_published><domain>example.test</domain><p>none</p></policy_published>
<record><row><source_ip>198.51.100.5</source_ip><count>3</count><policy_evaluated><disposition>none</disposition><dkim>pass</dkim><spf>fail</spf></policy_evaluated></row><identifiers><header_from>example.test</header_from></identifiers><auth_results><dkim><domain>example.test</domain><selector>test</selector><result>pass</result><human_result></human_result></dkim></auth_results></record></feedback>`

const tlsJSONFixture = `{"organization-name":"Synthetic Receiver","date-range":{"start-datetime":"2026-11-23T00:00:00Z","end-datetime":"2026-11-23T23:59:59Z"},"contact-info":"mailto:synthetic@example.test","report-id":"sample-tls","policies":[{"policy":{"policy-type":"sts","policy-domain":"example.test","mx-host":["smtp.example.test"]},"summary":{"total-successful-session-count":24,"total-failure-session-count":2},"failure-details":[{"result-type":"certificate-expired","sending-mta-ip":"192.0.2.33","receiving-mx-hostname":"smtp.example.test","failed-session-count":2}]},{"policy":{"policy-type":"tlsa","policy-domain":"example.test"},"summary":{"total-successful-session-count":3,"total-failure-session-count":0}}]}`

func zipped(t *testing.T, filename string, n int, data string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for i := 0; i < n; i++ {
		f, err := w.Create(fmt.Sprintf("%d-%s", i, filename))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := io.WriteString(w, s); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func parseAtt(kind Kind, name string, b []byte) (parsed, error) {
	return parseParts(kind, []*parsemail.Attachment{{Filename: name, Content: b}})
}

func TestParseReportFormats(t *testing.T) {
	for _, c := range []struct {
		name    string
		content []byte
		total   int64
		pct     int
	}{
		{"google.zip", zipped(t, "entry.xml", 1, google), 12, 75},
		{"microsoft.xml.gz", gzipped(t, microsoft), 6, 100},
		{"yahoo.xml", []byte(yahoo), 3, 100},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := parseAtt(DMARCAggregate, c.name, c.content)
			if err != nil || len(p.DMARC) != 1 {
				t.Fatalf("%v: %+v", err, p)
			}
			r := p.DMARC[0]
			if r.Report.Domain != "example.test" || r.Report.Pct != c.pct || len(r.Records) != 1 || r.Records[0].Count != c.total {
				t.Fatal(r)
			}
			if c.total == 12 && (r.Records[0].DKIMResult != "fail" || r.Records[0].DKIMAuth != "pass" || !strings.Contains(r.Records[0].DKIMDetails, "other.example.test")) {
				t.Fatal(r.Records[0])
			}
			if c.total == 6 && (r.Records[0].EnvelopeFrom != "relay.example.test" || r.Records[0].SourceIP != "2001:db8::2") {
				t.Fatal(r.Records[0])
			}
		})
	}
	p, err := parseAtt(TLSReport, "rpt.json.gz", gzipped(t, tlsJSONFixture))
	if err != nil || len(p.TLS) != 1 || p.TLS[0].SuccessCount != 27 || p.TLS[0].FailureCount != 2 || p.TLS[0].PolicyType != "sts,tlsa" || !strings.Contains(p.TLS[0].FailureDetails, "certificate-expired") {
		t.Fatalf("%v %+v", err, p.TLS)
	}
	long := strings.Replace(tlsJSONFixture, `{"result-type":"certificate-expired","sending-mta-ip":"192.0.2.33","receiving-mx-hostname":"smtp.example.test","failed-session-count":2}`, strings.Repeat(`{"result-type":"certificate-expired","failed-session-count":1},`, 55)+`{"result-type":"certificate-expired","failed-session-count":1}`, 1)
	p, err = parseAtt(TLSReport, "rpt.json", []byte(long))
	if err != nil || strings.Count(p.TLS[0].FailureDetails, "certificate-expired") != 50 {
		t.Fatal(err)
	}
}

func TestRejectUnsafeArchivesAndXML(t *testing.T) {
	big := strings.Repeat("a", unpackedLimit+1)
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"zip-bomb.zip", zipped(t, "report.xml", 1, big)},
		{"gzip-bomb.gz", gzipped(t, big)},
		{"too-many.zip", zipped(t, "report.xml", 11, google)},
		{"xxe.xml", []byte(`<!DOCTYPE feedback [<!ENTITY secret SYSTEM "file:///etc/passwd">]>` + strings.Replace(yahoo, "Yahoo Fixture", "&secret;", 1)[21:])},
		{"nested.xml", []byte(strings.Replace(yahoo, "</feedback>", strings.Repeat("<extension>", xmlDepth)+strings.Repeat("</extension>", xmlDepth)+"</feedback>", 1))},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseAtt(DMARCAggregate, c.name, c.data); err == nil {
				t.Fatal("unsafe payload accepted")
			}
		})
	}
}

func TestDSNWithMisleadingReportMetadata(t *testing.T) {
	e := &parsemail.Email{Subject: "DMARC Aggregate Report", Attachments: att("google.test!example.test!1795392000!1795478399.zip", "application/zip")}
	if Detect(e, []byte("Content-Type: multipart/report; report-type=delivery-status\r\n\r\n")) != None {
		t.Fatal("DSN filed by subject")
	}
	e.Attachments = append(e.Attachments, &parsemail.Attachment{ContentType: "message/delivery-status"})
	if Detect(e, nil) != None {
		t.Fatal("stored DSN filed by attachment")
	}
}
