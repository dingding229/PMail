// Package reportmail recognizes machine-generated mail reports, such as DMARC
// and SMTP TLS aggregate reports, so they can be filed away from the inbox.
// Delivery failure notices (DSN) are not reports here: users need to see them.
package reportmail

import (
	"bufio"
	"bytes"
	"mime"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

type Kind string

const (
	None           Kind = ""
	DMARCAggregate Kind = "dmarc_aggregate"
	DMARCFailure   Kind = "dmarc_failure"
	TLSReport      Kind = "tls_report"
	AbuseFeedback  Kind = "abuse_feedback"
)

const headerLimit = 64 << 10

// RFC 7489 section 7.2.1.1 subject, also used by Microsoft and Yahoo.
var dmarcSubject = regexp.MustCompile(`(?i)^\s*(?:\[[^\]]*\]\s*)?(?:dmarc\s+aggregate\s+report\b|report\s+domain:\s*\S+\s+submitter:\s*\S+)`)
var tlsSubject = regexp.MustCompile(`(?i)^\s*(?:\[[^\]]*\]\s*)?(?:smtp\s+)?tls(?:-?rpt|\s+report)\b.*\b(?:report\s+domain|submitter)\b`)

// <receiver>!<policy domain>!<begin>!<end>[!<id>].<ext>, RFC 7489 7.2.1.1 and RFC 8460 5.3.
var reportFilename = regexp.MustCompile(`(?i)^[^!/\\]+![^!/\\]+![0-9]{9,11}![0-9]{9,11}(?:![^./\\]*)?\.(xml|xml\.gz|zip|gz|json|json\.gz)$`)

// Detect classifies one received message. raw may be nil; headers are then
// taken from what parsemail kept.
func Detect(email *parsemail.Email, raw []byte) Kind {
	if email == nil {
		return None
	}
	h := topHeaders(raw)
	ctype, params, _ := mime.ParseMediaType(h.Get("Content-Type"))
	if ctype == "multipart/report" {
		switch strings.ToLower(params["report-type"]) {
		case "tlsrpt":
			return TLSReport
		case "feedback-report":
			return feedbackKind(raw)
		}
	}
	if h.Get("TLS-Report-Domain") != "" || h.Get("TLS-Report-Submitter") != "" {
		return TLSReport
	}
	for _, a := range email.Attachments {
		if a == nil {
			continue
		}
		at, _, _ := mime.ParseMediaType(a.ContentType)
		if at == "application/tlsrpt+gzip" || at == "application/tlsrpt+json" {
			return TLSReport
		}
		if at == "message/feedback-report" {
			return feedbackKind(raw)
		}
		if m := reportFilename.FindStringSubmatch(a.Filename); m != nil {
			if strings.HasPrefix(strings.ToLower(m[1]), "json") {
				return TLSReport
			}
			return DMARCAggregate
		}
	}
	if tlsSubject.MatchString(email.Subject) {
		return TLSReport
	}
	if dmarcSubject.MatchString(email.Subject) {
		return DMARCAggregate
	}
	return None
}

var feedbackType = regexp.MustCompile(`(?im)^feedback-type:\s*([a-z-]+)`)

func feedbackKind(raw []byte) Kind {
	if m := feedbackType.FindSubmatch(raw[:min(len(raw), 4*headerLimit)]); m != nil && strings.EqualFold(string(m[1]), "auth-failure") {
		return DMARCFailure
	}
	return AbuseFeedback
}

func topHeaders(raw []byte) textproto.MIMEHeader {
	if len(raw) == 0 {
		return textproto.MIMEHeader{}
	}
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw[:min(len(raw), headerLimit)])))
	h, _ := r.ReadMIMEHeader()
	if h == nil {
		return textproto.MIMEHeader{}
	}
	return h
}
