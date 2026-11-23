// Package inbound scores incoming mail and scans untrusted content as data only.
package inbound

import (
	"database/sql"
	"encoding/json"
	"net/netip"
	"sync"

	"github.com/Jinnrry/pmail/services/antivirus"
)

type Envelope struct {
	SourceIP netip.Addr // Effective connection IP after trusted-relay processing.
	MailFrom string
}

type Reason struct {
	Code   string  `json:"code"`
	Weight float64 `json:"weight"`
	Detail string  `json:"detail,omitempty"`
}
type Spam struct {
	Score   float64  `json:"score"`
	Verdict string   `json:"verdict"`
	Reasons []Reason `json:"reasons"`
}
type Finding struct {
	Code            string `json:"code"`
	Severity        string `json:"severity"`
	Location        string `json:"location"`
	AttachmentIndex *int   `json:"attachment_index,omitempty"`
	Line            int    `json:"line,omitempty"`
	Excerpt         string `json:"excerpt,omitempty"`
}
type Risk struct {
	Level    string    `json:"level"`
	Findings []Finding `json:"findings"`
}
type VerificationCode struct {
	Code       string  `json:"code"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
	Kind       string  `json:"kind,omitempty"`
}

type Analysis struct {
	Antivirus        *antivirus.Result `json:"antivirus,omitempty"`
	Version          int               `json:"version"`
	Risk             *Risk             `json:"risk"`
	VerificationCode *VerificationCode `json:"verification_code"`
	Base             []Reason          `json:"base"`
	HighRiskParts    []int             `json:"high_risk_parts,omitempty"` // authoritative bounded list, independent of findings cap
	Recipient        *Spam             `json:"-"`
}

type DetailView struct {
	Antivirus        *antivirus.Result `json:"antivirus,omitempty"`
	Spam             *Spam             `json:"spam"`
	Risk             *Risk             `json:"risk"`
	VerificationCode *VerificationCode `json:"verification_code"`
}
type SpamSummary struct {
	Verdict string `json:"verdict"`
}
type RiskSummary struct {
	Level string `json:"level"`
}
type VerificationSummary struct {
	Code string `json:"code"`
	Kind string `json:"kind,omitempty"`
}
type ListView struct {
	Spam             *SpamSummary         `json:"spam"`
	Risk             *RiskSummary         `json:"risk"`
	VerificationCode *VerificationSummary `json:"verification_code"`
}

func Storage(v any) sql.NullString {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}
func FromStorage(v sql.NullString) *Analysis {
	if !v.Valid {
		return nil
	}
	var a *Analysis
	if json.Unmarshal([]byte(v.String), &a) != nil {
		return nil
	}
	return a
}
func ReadDetail(shared, personal sql.NullString) DetailView {
	var v DetailView
	if a := FromStorage(shared); a != nil {
		v.Risk = a.Risk
		v.Antivirus = a.Antivirus
		v.VerificationCode = a.VerificationCode
	}
	if personal.Valid {
		_ = json.Unmarshal([]byte(personal.String), &v.Spam)
	}
	return v
}
func ReadList(shared, personal sql.NullString) ListView {
	v := ReadDetail(shared, personal)
	var out ListView
	if v.Spam != nil {
		out.Spam = &SpamSummary{v.Spam.Verdict}
	}
	if v.Risk != nil {
		out.Risk = &RiskSummary{v.Risk.Level}
	}
	if v.VerificationCode != nil {
		out.VerificationCode = &VerificationSummary{Code: v.VerificationCode.Code, Kind: v.VerificationCode.Kind}
	}
	return out
}

var notifier struct {
	sync.RWMutex
	send func(int, int, *Analysis)
}

func SetNotifier(fn func(int, int, *Analysis)) {
	notifier.Lock()
	notifier.send = fn
	notifier.Unlock()
}

// Notify executes only after durable delivery; consumers must not reject mail.
func Notify(userID, emailID int, analysis *Analysis) {
	notifier.RLock()
	fn := notifier.send
	notifier.RUnlock()
	if fn != nil {
		fn(userID, emailID, analysis)
	}
}
