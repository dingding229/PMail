package reportmail

import (
	"testing"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

func att(name, ctype string) []*parsemail.Attachment {
	return []*parsemail.Attachment{{Filename: name, ContentType: ctype}}
}

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name    string
		subject string
		atts    []*parsemail.Attachment
		raw     string
		want    Kind
	}{
		{"google dmarc", "Report domain: xshi.cc Submitter: google.com Report-ID: 2556873338071705901", att("google.com!xshi.cc!1795392000!1795478399.zip", "application/zip"), "", DMARCAggregate},
		{"microsoft dmarc", "Report Domain: xshi.cc Submitter: enterprise.protection.outlook.com Report-ID: <abc>", att("enterprise.protection.outlook.com!xshi.cc!1795392000!1795478400.xml.gz", "application/gzip"), "", DMARCAggregate},
		{"dmarc by file name only", "Weekly summary", att("yahoo.com!xshi.cc!1795392000!1795478399.xml.gz", "application/gzip"), "", DMARCAggregate},
		{"dmarc subject only", "[Preview] DMARC Aggregate Report for xshi.cc", nil, "", DMARCAggregate},
		{"tls report by content type", "Report Domain: xshi.cc Submitter: google.com Report-ID: <1@google.com>", att("google.com!xshi.cc!1795392000!1795478399!001.json.gz", "application/tlsrpt+gzip"), "", TLSReport},
		{"tls report header", "Anything", nil, "TLS-Report-Domain: xshi.cc\r\nTLS-Report-Submitter: google.com\r\nContent-Type: text/plain\r\n\r\nx", TLSReport},
		{"tls multipart/report", "Report", nil, "Content-Type: multipart/report; report-type=tlsrpt; boundary=x\r\n\r\n--x", TLSReport},
		{"dmarc failure", "Auth failure", nil, "Content-Type: multipart/report; report-type=feedback-report; boundary=x\r\n\r\n--x\r\nContent-Type: message/feedback-report\r\n\r\nFeedback-Type: auth-failure\r\n", DMARCFailure},
		{"abuse feedback", "Complaint", nil, "Content-Type: multipart/report; report-type=feedback-report; boundary=x\r\n\r\n--x\r\nContent-Type: message/feedback-report\r\n\r\nFeedback-Type: abuse\r\n", AbuseFeedback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(&parsemail.Email{Subject: tc.subject, Attachments: tc.atts}, []byte(tc.raw)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectLeavesOrdinaryMail(t *testing.T) {
	for _, tc := range []struct {
		name    string
		subject string
		atts    []*parsemail.Attachment
		raw     string
	}{
		{"bounce stays visible", "Undelivered Mail Returned to Sender", nil, "Content-Type: multipart/report; report-type=delivery-status; boundary=x\r\n\r\n--x"},
		{"person mentions dmarc", "Question about our DMARC report setup", nil, ""},
		{"ordinary zip", "Photos", att("holiday.zip", "application/zip"), ""},
		{"report word in subject", "Monthly report: domain sales", nil, ""},
		{"exclamation file name", "Hi", att("wow!this!is!fun.zip", "application/zip"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(&parsemail.Email{Subject: tc.subject, Attachments: tc.atts}, []byte(tc.raw)); got != None {
				t.Fatalf("got %q, want none", got)
			}
		})
	}
}
