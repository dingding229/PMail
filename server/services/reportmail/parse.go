package reportmail

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"net/netip"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
)

const (
	unpackedLimit = 20 << 20
	zipLimit      = 10
	xmlDepth      = 32
	detailLimit   = 50
	maxCount      = 1_000_000_000_000
	maxUnix       = 253402300799 // Last second representable as a four-digit RFC 3339 year.
)

var errPayload = errors.New("invalid or oversized mail report")

type aggregate struct {
	Report  models.DMARCReport
	Records []models.DMARCRecord
}

type parsed struct {
	DMARC []aggregate
	TLS   []models.TLSReport
}

func candidate(a *parsemail.Attachment, kind Kind) bool {
	if a == nil {
		return false
	}
	t, _, _ := mime.ParseMediaType(a.ContentType)
	switch t {
	case "application/zip", "application/x-zip-compressed", "application/gzip", "application/x-gzip":
		return true
	case "application/tlsrpt+gzip", "application/tlsrpt+json", "application/json":
		return kind == TLSReport
	case "application/xml", "text/xml":
		return kind == DMARCAggregate
	}
	n := strings.ToLower(a.Filename)
	return strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".gz") ||
		(kind == DMARCAggregate && strings.HasSuffix(n, ".xml")) || (kind == TLSReport && strings.HasSuffix(n, ".json"))
}

// parseParts shares one budget across all attachments, including plain XML/JSON.
// No archive path is ever used as a filesystem path; nested archives aren't decoded.
func parseParts(kind Kind, parts []*parsemail.Attachment) (out parsed, err error) {
	left := int64(unpackedLimit)
	files := 0
	accept := func(r io.Reader) error {
		files++
		if files > zipLimit || left <= 0 {
			return errPayload
		}
		b, err := io.ReadAll(io.LimitReader(r, left+1))
		if err != nil || int64(len(b)) > left {
			return errPayload
		}
		left -= int64(len(b))
		if kind == TLSReport {
			rows, err := parseTLS(b)
			out.TLS = append(out.TLS, rows...)
			return err
		}
		row, err := parseDMARC(b)
		if err == nil {
			out.DMARC = append(out.DMARC, row)
		}
		return err
	}
	for _, a := range parts {
		if !candidate(a, kind) {
			continue
		}
		switch {
		case bytes.HasPrefix(a.Content, []byte("PK\x03\x04")), bytes.HasPrefix(a.Content, []byte("PK\x05\x06")):
			z, err := zip.NewReader(bytes.NewReader(a.Content), int64(len(a.Content)))
			if err != nil || len(z.File) > zipLimit {
				return parsed{}, errPayload
			}
			var declared uint64
			for _, f := range z.File {
				if f.UncompressedSize64 > uint64(left)-declared {
					return parsed{}, errPayload
				}
				declared += f.UncompressedSize64
			}
			for _, f := range z.File {
				ext := strings.ToLower(path.Ext(f.Name))
				if f.FileInfo().IsDir() || (kind == TLSReport && ext != ".json") || (kind == DMARCAggregate && ext != ".xml") {
					continue
				}
				r, err := f.Open()
				if err != nil {
					return parsed{}, errPayload
				}
				err = accept(r)
				_ = r.Close()
				if err != nil {
					return parsed{}, err
				}
			}
		case bytes.HasPrefix(a.Content, []byte{0x1f, 0x8b}):
			r, err := gzip.NewReader(bytes.NewReader(a.Content))
			if err != nil {
				return parsed{}, errPayload
			}
			err = accept(r)
			_ = r.Close()
			if err != nil {
				return parsed{}, err
			}
		default:
			if err := accept(bytes.NewReader(a.Content)); err != nil {
				return parsed{}, err
			}
		}
	}
	if files == 0 {
		return parsed{}, errPayload
	}
	return out, nil
}

// Go's XML decoder has no external resolver. Explicitly reject all directives,
// including DTDs, before decoding any nested values.
func checkXML(b []byte) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	depth, roots := 0, 0
	for {
		t, err := d.Token()
		if err == io.EOF && roots == 1 && depth == 0 {
			return nil
		}
		if err != nil {
			return errPayload
		}
		switch t := t.(type) {
		case xml.Directive:
			return errPayload
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
			if roots > 1 || depth > xmlDepth {
				return errPayload
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(t)) != 0 {
				return errPayload
			}
		}
	}
}

type authDetail struct {
	Domain   string `xml:"domain" json:"domain"`
	Selector string `xml:"selector" json:"selector,omitempty"`
	Result   string `xml:"result" json:"result"`
}

type dmarcXML struct {
	XMLName xml.Name `xml:"feedback"`
	Org     string   `xml:"report_metadata>org_name"`
	ID      string   `xml:"report_metadata>report_id"`
	Begin   int64    `xml:"report_metadata>date_range>begin"`
	End     int64    `xml:"report_metadata>date_range>end"`
	Domain  string   `xml:"policy_published>domain"`
	P       string   `xml:"policy_published>p"`
	SP      string   `xml:"policy_published>sp"`
	Pct     *int     `xml:"policy_published>pct"`
	ADKIM   string   `xml:"policy_published>adkim"`
	ASPF    string   `xml:"policy_published>aspf"`
	Records []struct {
		IP           string       `xml:"row>source_ip"`
		Count        int64        `xml:"row>count"`
		Disposition  string       `xml:"row>policy_evaluated>disposition"`
		DKIM         string       `xml:"row>policy_evaluated>dkim"`
		SPF          string       `xml:"row>policy_evaluated>spf"`
		HeaderFrom   string       `xml:"identifiers>header_from"`
		EnvelopeFrom string       `xml:"identifiers>envelope_from"`
		DKIMDetails  []authDetail `xml:"auth_results>dkim"`
		SPFDetails   []authDetail `xml:"auth_results>spf"`
	} `xml:"record"`
}

func parseDMARC(b []byte) (aggregate, error) {
	var x dmarcXML
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	if checkXML(b) != nil || xml.Unmarshal(b, &x) != nil {
		return aggregate{}, errPayload
	}
	d, err := domain.Normalize(x.Domain)
	org, id := identity(x.Org, 160), identity(x.ID, 256)
	if err != nil || org == "" || id == "" || !validTimes(x.Begin, x.End) || len(x.Records) > 100_000 {
		return aggregate{}, errPayload
	}
	p, sp, pct := token(x.P, 16), token(x.SP, 16), 100
	if p != "none" && p != "quarantine" && p != "reject" {
		return aggregate{}, errPayload
	}
	if sp == "" {
		sp = p
	}
	if x.Pct != nil {
		pct = *x.Pct
		if pct < 0 || pct > 100 {
			return aggregate{}, errPayload
		}
	}
	alignment := func(s string) string {
		if token(s, 1) == "s" {
			return "s"
		}
		return "r"
	}
	out := aggregate{Report: models.DMARCReport{Domain: d, OrgName: org, ReportID: id, BeginAt: x.Begin, EndAt: x.End,
		BeginDay: date(x.Begin), P: p, SP: sp, Pct: pct, ADKIM: alignment(x.ADKIM), ASPF: alignment(x.ASPF)}}
	var sum int64
	for _, r := range x.Records {
		a, err := netip.ParseAddr(strings.TrimSpace(r.IP))
		if err != nil || a.Zone() != "" || r.Count <= 0 || r.Count > maxCount-sum {
			return aggregate{}, errPayload
		}
		sum += r.Count
		header, _ := domain.Normalize(r.HeaderFrom)
		dkim, dkimJSON := details(r.DKIMDetails)
		spf, spfJSON := details(r.SPFDetails)
		out.Records = append(out.Records, models.DMARCRecord{SourceIP: a.Unmap().String(), Count: r.Count, Disposition: token(r.Disposition, 16),
			DKIMResult: token(r.DKIM, 16), SPFResult: token(r.SPF, 16), HeaderFrom: header, EnvelopeFrom: clean(r.EnvelopeFrom, 320),
			DKIMDomain: dkim.Domain, DKIMSelector: dkim.Selector, DKIMAuth: dkim.Result, DKIMDetails: dkimJSON, SPFDomain: spf.Domain, SPFAuth: spf.Result, SPFDetails: spfJSON})
	}
	return out, nil
}

func details(in []authDetail) (authDetail, string) {
	in = in[:min(len(in), detailLimit)]
	for i := range in {
		in[i] = authDetail{clean(in[i].Domain, 253), clean(in[i].Selector, 253), token(in[i].Result, 16)}
	}
	if len(in) == 0 {
		return authDetail{}, "[]"
	}
	return in[0], jsonText(in)
}

type tlsFailure struct {
	ResultType  string `json:"result-type"`
	Count       int64  `json:"failed-session-count"`
	SendingIP   string `json:"sending-mta-ip,omitempty"`
	ReceivingIP string `json:"receiving-ip,omitempty"`
	ReceivingMX string `json:"receiving-mx-hostname,omitempty"`
	Reason      string `json:"failure-reason-code,omitempty"`
}

type tlsJSON struct {
	Organization string `json:"organization-name"`
	ID           string `json:"report-id"`
	Dates        struct {
		Start time.Time `json:"start-datetime"`
		End   time.Time `json:"end-datetime"`
	} `json:"date-range"`
	Policies []struct {
		Policy struct {
			Type   string `json:"policy-type"`
			Domain string `json:"policy-domain"`
		} `json:"policy"`
		Summary struct {
			Success int64 `json:"total-successful-session-count"`
			Failure int64 `json:"total-failure-session-count"`
		} `json:"summary"`
		Details []tlsFailure `json:"failure-details"`
	} `json:"policies"`
}

func parseTLS(b []byte) ([]models.TLSReport, error) {
	var x tlsJSON
	if json.Unmarshal(b, &x) != nil {
		return nil, errPayload
	}
	org, id := identity(x.Organization, 160), identity(x.ID, 256)
	start, end := x.Dates.Start.Unix(), x.Dates.End.Unix()
	if org == "" || id == "" || !validTimes(start, end) || len(x.Policies) == 0 || len(x.Policies) > 1000 {
		return nil, errPayload
	}
	type bundle struct {
		row          models.TLSReport
		types        []string
		details      []tlsFailure
		failureTypes map[string]int64
	}
	byDomain := make(map[string]*bundle)
	for _, p := range x.Policies {
		d, err := domain.Normalize(p.Policy.Domain)
		k := strings.ToLower(strings.TrimSpace(p.Policy.Type))
		if err != nil || (k != "sts" && k != "tlsa" && k != "no-policy-found") {
			return nil, errPayload
		}
		v := byDomain[d]
		if v == nil {
			v = &bundle{row: models.TLSReport{Domain: d, OrgName: org, ReportID: id, BeginAt: start, EndAt: end}, details: []tlsFailure{}, failureTypes: make(map[string]int64)}
			byDomain[d] = v
		}
		if !strings.Contains(","+strings.Join(v.types, ",")+",", ","+k+",") {
			v.types = append(v.types, k)
		}
		a, z := p.Summary.Success, p.Summary.Failure
		if a < 0 || z < 0 || a > maxCount-v.row.SuccessCount || z > maxCount-v.row.FailureCount {
			return nil, errPayload
		}
		v.row.SuccessCount += a
		v.row.FailureCount += z
		for _, f := range p.Details {
			if f.Count < 0 || f.Count > maxCount {
				return nil, errPayload
			}
			resultType := token(f.ResultType, 64)
			if resultType != "" {
				if _, known := v.failureTypes[resultType]; !known && len(v.failureTypes) >= 100 {
					resultType = "other"
				}
				if f.Count > maxCount-v.failureTypes[resultType] {
					return nil, errPayload
				}
				v.failureTypes[resultType] += f.Count
			}
			if len(v.details) < detailLimit {
				v.details = append(v.details, tlsFailure{resultType, f.Count, clean(f.SendingIP, 45), clean(f.ReceivingIP, 45), clean(f.ReceivingMX, 253), clean(f.Reason, 160)})
			}
		}
	}
	out := make([]models.TLSReport, 0, len(byDomain))
	for _, v := range byDomain {
		v.row.PolicyType = strings.Join(v.types, ",")
		v.row.FailureDetails = jsonText(v.details)
		v.row.FailureTypes = jsonText(v.failureTypes)
		out = append(out, v.row)
	}
	return out, nil
}

func validTimes(start, end int64) bool { return start > 0 && end >= start && end <= maxUnix }
func date(t int64) string              { return time.Unix(t, 0).UTC().Format(time.DateOnly) }

// Reject overlong identity keys instead of making unrelated reports collide.
func identity(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n || strings.ContainsFunc(s, unicode.IsControl) {
		return ""
	}
	return s
}

func clean(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) <= n {
		return s
	}
	for !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
func token(s string, n int) string { return strings.ToLower(clean(s, n)) }
func jsonText(v any) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false) // Raw JSON storage; never used as an HTML fragment.
	_ = e.Encode(v)        // All callers provide plain scalars/slices of scalar structs.
	return strings.TrimSpace(b.String())
}
