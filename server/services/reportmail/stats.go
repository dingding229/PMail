package reportmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
)

var ErrQuery = errors.New("invalid mail report query")

type Window struct {
	Domain string
	From   time.Time
	To     time.Time // inclusive calendar day, in UTC
}

func ResolveWindow(ctx context.Context, name, from, to string, now time.Time) (w Window, err error) {
	if name == "" {
		name = domain.DefaultName()
	}
	if name != "" {
		w.Domain, err = domain.Normalize(name)
		if err != nil {
			return Window{}, ErrQuery
		}
		ok, err := db.Instance.Context(ctx).Where("name=?", w.Domain).Exist(&models.Domain{})
		if err != nil {
			return Window{}, err
		}
		if !ok {
			return Window{}, ErrQuery
		}
	}
	w.To = now.UTC().Truncate(24 * time.Hour)
	if to != "" {
		w.To, err = time.Parse(time.DateOnly, to)
		if err != nil {
			return Window{}, ErrQuery
		}
	}
	w.From = w.To.AddDate(0, 0, -29)
	if from != "" {
		w.From, err = time.Parse(time.DateOnly, from)
		if err != nil {
			return Window{}, ErrQuery
		}
	}
	if w.From.After(w.To) || w.To.Sub(w.From) >= Retention || w.From.Year() < 1970 || w.To.Year() > 9998 {
		return Window{}, ErrQuery
	}
	return w, nil
}
func (w Window) args() []any { return []any{w.Domain, w.From.Unix(), w.To.AddDate(0, 0, 1).Unix()} }

const whereDMARC = `r.domain=? AND r.begin_at>=? AND r.begin_at<?`
const whereTLS = `domain=? AND begin_at>=? AND begin_at<?`
const joined = ` FROM dmarc_record d JOIN dmarc_report r ON d.report_id=r.id WHERE ` + whereDMARC
const passed = `(d.dkim_result='pass' OR d.spf_result='pass')`
const passSum = `COALESCE(SUM(CASE WHEN ` + passed + ` THEN d.mail_count ELSE 0 END),0)`

func pct(n, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(n)/float64(total)*10000) / 100
}

type Totals struct {
	Total         int64   `xorm:"total" json:"total"`
	DMARCPass     int64   `xorm:"dmarc_pass" json:"dmarc_pass"`
	SPFPass       int64   `xorm:"spf_pass" json:"spf_pass"`
	DKIMPass      int64   `xorm:"dkim_pass" json:"dkim_pass"`
	Quarantined   int64   `json:"quarantined"`
	Rejected      int64   `json:"rejected"`
	DMARCPassRate float64 `json:"dmarc_pass_rate"`
	SPFPassRate   float64 `json:"spf_pass_rate"`
	DKIMPassRate  float64 `json:"dkim_pass_rate"`
}
type Day struct {
	Date string `json:"date"`
	Pass int64  `json:"pass"`
	Fail int64  `json:"fail"`
}
type ByReporter struct {
	OrgName   string  `json:"org_name"`
	Count     int64   `xorm:"qty" json:"count"`
	PassCount int64   `json:"pass_count"`
	PassRate  float64 `json:"pass_rate"`
}
type BySource struct {
	SourceIP        string  `xorm:"source_ip" json:"source_ip"`
	Count           int64   `xorm:"qty" json:"count"`
	PassCount       int64   `json:"pass_count"`
	PassRate        float64 `json:"pass_rate"`
	IsOwn           bool    `json:"is_own"`
	Suspicious      bool    `json:"suspicious"`
	ClaimedFailures int64   `json:"-"`
}
type FailureType struct {
	ResultType string `json:"result_type"`
	Count      int64  `json:"count"`
}
type TLSSummary struct {
	SuccessCount int64         `json:"success_count"`
	FailureCount int64         `json:"failure_count"`
	ReportCount  int64         `json:"report_count"`
	FailureTypes []FailureType `json:"failure_types"`
}
type Policy struct {
	P          string `json:"p"`
	SP         string `json:"sp"`
	Pct        int    `json:"pct"`
	ADKIM      string `json:"adkim"`
	ASPF       string `json:"aspf"`
	ReportDate string `json:"report_date"`
	OrgName    string `json:"org_name"`
}
type SummaryResult struct {
	Domain         string       `json:"domain"`
	From           string       `json:"from"`
	To             string       `json:"to"`
	ReportCount    int64        `json:"report_count"`
	LatestReportAt string       `json:"latest_report_at,omitempty"`
	LatestPolicy   *Policy      `json:"latest_policy"`
	Totals         Totals       `json:"totals"`
	Daily          []Day        `json:"daily"`
	Reporters      []ByReporter `json:"reporters"`
	Sources        []BySource   `json:"sources"`
	TLS            *TLSSummary  `json:"tls"`
}

type OwnLookup func(ctx context.Context, name string, ips []netip.Addr) map[netip.Addr]bool

func Summary(ctx context.Context, w Window, lookup OwnLookup) (s SummaryResult, err error) {
	s = SummaryResult{Domain: w.Domain, From: w.From.Format(time.DateOnly), To: w.To.Format(time.DateOnly), Daily: []Day{}, Reporters: []ByReporter{}, Sources: []BySource{}}
	a := w.args()
	var r models.DMARCReport
	if found, err := db.Instance.Context(ctx).Where(whereTLS, a...).Desc("begin_at", "id").Get(&r); err != nil {
		return s, err
	} else if found {
		s.LatestReportAt = time.Unix(r.BeginAt, 0).UTC().Format(time.RFC3339)
		s.LatestPolicy = &Policy{r.P, r.SP, r.Pct, r.ADKIM, r.ASPF, r.BeginDay, r.OrgName}
	}
	s.ReportCount, err = db.Instance.Context(ctx).Where(whereTLS, a...).Count(&models.DMARCReport{})
	if err != nil {
		return s, err
	}
	get := `SELECT COALESCE(SUM(d.mail_count),0) AS total, ` + passSum + ` AS dmarc_pass,
		COALESCE(SUM(CASE WHEN d.spf_result='pass' THEN d.mail_count ELSE 0 END),0) AS spf_pass,
		COALESCE(SUM(CASE WHEN d.dkim_result='pass' THEN d.mail_count ELSE 0 END),0) AS dkim_pass,
		COALESCE(SUM(CASE WHEN d.disposition='quarantine' THEN d.mail_count ELSE 0 END),0) AS quarantined,
		COALESCE(SUM(CASE WHEN d.disposition='reject' THEN d.mail_count ELSE 0 END),0) AS rejected` + joined
	if _, err := db.Instance.Context(ctx).SQL(get, a...).Get(&s.Totals); err != nil {
		return s, err
	}
	t := &s.Totals
	t.DMARCPassRate, t.SPFPassRate, t.DKIMPassRate = pct(t.DMARCPass, t.Total), pct(t.SPFPass, t.Total), pct(t.DKIMPass, t.Total)
	if err := db.Instance.Context(ctx).SQL(`SELECT r.begin_day AS date, `+passSum+` AS pass, SUM(d.mail_count)-`+passSum+` AS fail`+joined+` GROUP BY r.begin_day ORDER BY r.begin_day`, a...).Find(&s.Daily); err != nil {
		return s, err
	}
	if len(s.Daily) > 0 {
		points := make(map[string]Day, len(s.Daily))
		for _, d := range s.Daily {
			points[d.Date] = d
		}
		s.Daily = s.Daily[:0]
		for d := w.From; !d.After(w.To); d = d.AddDate(0, 0, 1) {
			key := d.Format(time.DateOnly)
			v := points[key]
			v.Date = key
			s.Daily = append(s.Daily, v)
		}
	}

	if err := db.Instance.Context(ctx).SQL(`SELECT r.org_name, SUM(d.mail_count) AS qty, `+passSum+` AS pass_count`+joined+` GROUP BY r.org_name ORDER BY qty DESC,r.org_name`, a...).Find(&s.Reporters); err != nil {
		return s, err
	}
	qa := append([]any{w.Domain, "%." + w.Domain}, a...)
	if err := db.Instance.Context(ctx).SQL(`SELECT d.source_ip, SUM(d.mail_count) AS qty, `+passSum+` AS pass_count,
		SUM(CASE WHEN NOT `+passed+` AND (d.header_from=? OR d.header_from LIKE ?) THEN d.mail_count ELSE 0 END) AS claimed_failures`+joined+` GROUP BY d.source_ip ORDER BY qty DESC,d.source_ip LIMIT 50`, qa...).Find(&s.Sources); err != nil {
		return s, err
	}
	for i := range s.Reporters {
		r := &s.Reporters[i]
		r.PassRate = pct(r.PassCount, r.Count)
	}
	s.TLS, err = tlsSummary(ctx, w)
	if err != nil {
		return s, err
	}
	if lookup == nil {
		lookup = lookupOwn
	}
	var ips []netip.Addr
	for _, v := range s.Sources {
		if ip, err := netip.ParseAddr(v.SourceIP); err == nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) > 0 {
		own := lookup(ctx, w.Domain, ips)
		for i := range s.Sources {
			v := &s.Sources[i]
			ip, _ := netip.ParseAddr(v.SourceIP)
			v.PassRate, v.IsOwn = pct(v.PassCount, v.Count), own[ip]
			v.Suspicious = !v.IsOwn && v.ClaimedFailures > 0
		}
	}
	return s, nil
}

func lookupOwn(ctx context.Context, name string, ips []netip.Addr) map[netip.Addr]bool {
	out := map[netip.Addr]bool{}
	for _, e := range []string{"PMAIL_PUBLIC_IPV4", "PMAIL_PUBLIC_IPV6"} {
		if ip, err := netip.ParseAddr(strings.TrimSpace(os.Getenv(e))); err == nil {
			out[ip.Unmap()] = true
		}
	}
	r, err := domain.Resolver()
	if err != nil || name == "" {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var lock sync.Mutex
	var wg sync.WaitGroup
	for chunk := range slices.Chunk(ips, 6) {
		if ctx.Err() != nil {
			break
		}
		for _, ip := range chunk {
			lock.Lock()
			known := out[ip]
			lock.Unlock()
			if known {
				continue
			}
			wg.Go(func() {
				if domain.ReportIPAuthorized(ctx, r, name, ip) {
					lock.Lock()
					out[ip] = true
					lock.Unlock()
				}
			})
		}
		wg.Wait()
	}
	return out
}

func tlsSummary(ctx context.Context, w Window) (*TLSSummary, error) {
	s := &TLSSummary{FailureTypes: []FailureType{}}
	all := map[string]int64{}
	rows, err := db.Instance.Context(ctx).Where(whereTLS, w.args()...).Cols("success_count", "failure_count", "failure_types").Rows(&models.TLSReport{})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r models.TLSReport
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		s.ReportCount++
		s.SuccessCount += r.SuccessCount
		s.FailureCount += r.FailureCount
		var fs map[string]int64
		if json.Unmarshal([]byte(r.FailureTypes), &fs) == nil {
			for kind, count := range fs {
				all[kind] += count
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if s.ReportCount == 0 {
		return nil, nil
	}
	for k, n := range all {
		s.FailureTypes = append(s.FailureTypes, FailureType{k, n})
	}
	slices.SortFunc(s.FailureTypes, func(a, b FailureType) int {
		if a.Count > b.Count {
			return -1
		}
		if a.Count < b.Count {
			return 1
		}
		return strings.Compare(a.ResultType, b.ResultType)
	})
	s.FailureTypes = s.FailureTypes[:min(10, len(s.FailureTypes))]
	return s, nil
}

type RecordItem struct {
	ID           int    `xorm:"id" json:"id"`
	Date         string `json:"date"`
	SourceIP     string `xorm:"source_ip" json:"source_ip"`
	Count        int64  `xorm:"mail_count" json:"count"`
	Disposition  string `json:"disposition"`
	HeaderFrom   string `json:"header_from"`
	EnvelopeFrom string `json:"envelope_from,omitempty"`
	DKIMResult   string `xorm:"dkim_result" json:"dkim_result"`
	DKIMDomain   string `xorm:"dkim_domain" json:"dkim_domain,omitempty"`
	DKIMSelector string `xorm:"dkim_selector" json:"dkim_selector,omitempty"`
	DKIMAuth     string `xorm:"dkim_auth" json:"dkim_auth_result,omitempty"`
	SPFResult    string `xorm:"spf_result" json:"spf_result"`
	SPFDomain    string `xorm:"spf_domain" json:"spf_domain,omitempty"`
	SPFAuth      string `xorm:"spf_auth" json:"spf_auth_result,omitempty"`
	OrgName      string `json:"org_name"`
	EmailID      int    `xorm:"email_id" json:"email_id,omitempty"`
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

const visibleID = `CASE WHEN EXISTS (SELECT 1 FROM user_email ue WHERE ue.email_id=r.email_id AND ue.user_id=?) THEN r.email_id ELSE 0 END AS email_id`

func Records(ctx context.Context, w Window, uid, page int, result string) (p Page[RecordItem], err error) {
	p = Page[RecordItem]{Items: []RecordItem{}, Page: page, PageSize: 10}
	filter := joined
	switch result {
	case "", "fail":
		filter += ` AND NOT ` + passed
	case "pass":
		filter += ` AND ` + passed
	case "all":
	default:
		return p, ErrQuery
	}
	if page < 1 || page > 1000000 {
		return p, ErrQuery
	}
	var count struct{ Total int64 }
	if _, err := db.Instance.Context(ctx).SQL(`SELECT COUNT(*) AS total`+filter, w.args()...).Get(&count); err != nil {
		return p, err
	}
	p.Total = count.Total
	q := `SELECT d.id,r.begin_day AS date,d.source_ip,d.mail_count,d.disposition,d.header_from,d.envelope_from,
		d.dkim_result,d.dkim_domain,d.dkim_selector,d.dkim_auth,d.spf_result,d.spf_domain,d.spf_auth,r.org_name,` + visibleID + filter + ` ORDER BY r.begin_at DESC,r.id DESC,d.id DESC LIMIT ? OFFSET ?`
	args := append([]any{uid}, w.args()...)
	args = append(args, p.PageSize, (page-1)*p.PageSize)
	err = db.Instance.Context(ctx).SQL(q, args...).Find(&p.Items)
	return
}

type ReportItem struct {
	ID       int       `xorm:"id" json:"id"`
	Kind     Kind      `json:"kind"`
	Domain   string    `json:"domain"`
	OrgName  string    `json:"org_name"`
	ReportID string    `xorm:"report_id" json:"report_id"`
	BeginAt  time.Time `xorm:"-" json:"begin_at"`
	EndAt    time.Time `xorm:"-" json:"end_at"`
	EmailID  int       `xorm:"email_id" json:"email_id,omitempty"`
}

func Reports(ctx context.Context, w Window, uid, page int) (p Page[ReportItem], err error) {
	p = Page[ReportItem]{Items: []ReportItem{}, Page: page, PageSize: 20}
	if page < 1 || page > 1000000 {
		return p, ErrQuery
	}
	for _, model := range []any{&models.DMARCReport{}, &models.TLSReport{}} {
		n, err := db.Instance.Context(ctx).Where(whereTLS, w.args()...).Count(model)
		if err != nil {
			return p, err
		}
		p.Total += n
	}
	selectSQL := `SELECT r.id, '%s' AS kind,r.domain,r.org_name,r.report_id,r.begin_at,r.end_at,` + visibleID + ` FROM %s r WHERE ` + whereDMARC
	q := fmt.Sprintf(selectSQL, DMARCAggregate, "dmarc_report") + ` UNION ALL ` +
		fmt.Sprintf(selectSQL, TLSReport, "tls_report") + ` ORDER BY begin_at DESC,kind,id DESC LIMIT ? OFFSET ?`
	args := append([]any{uid}, w.args()...)
	args = append(args, uid)
	args = append(args, w.args()...)
	args = append(args, p.PageSize, (page-1)*p.PageSize)
	var rows []struct {
		ID       int `xorm:"id"`
		Kind     string
		Domain   string
		OrgName  string
		ReportID string `xorm:"report_id"`
		BeginAt  int64
		EndAt    int64
		EmailID  int `xorm:"email_id"`
	}
	if err := db.Instance.Context(ctx).SQL(q, args...).Find(&rows); err != nil {
		return p, err
	}
	for _, r := range rows {
		p.Items = append(p.Items, ReportItem{r.ID, Kind(r.Kind), r.Domain, r.OrgName, r.ReportID, time.Unix(r.BeginAt, 0).UTC(), time.Unix(r.EndAt, 0).UTC(), r.EmailID})
	}
	return p, nil
}
