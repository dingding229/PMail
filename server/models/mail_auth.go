package models

import "encoding/json"

// MailAuthResults is the persisted transport evidence. TrustedRelay is
// separate from the pass/fail of each independent authentication check.
type MailAuthResults struct {
	SPF          MailAuthCheck `json:"spf"`
	DKIM         MailAuthCheck `json:"dkim"`
	DMARC        MailAuthCheck `json:"dmarc"`
	TrustedRelay bool          `json:"trusted_relay"`
}

type MailAuthCheck struct {
	Result  string `json:"result"`
	Reason  string `json:"reason"`
	Domain  string `json:"domain,omitempty"`
	Aligned bool   `json:"aligned,omitempty"`
	Policy  string `json:"policy,omitempty"`
}

func (m *MailAuthResults) Storage() string {
	if m == nil {
		return ""
	}
	v, _ := json.Marshal(m)
	return string(v)
}

func (d Email) TransportAuth() *MailAuthResults {
	if !d.AuthResults.Valid || d.AuthResults.String == "" {
		return nil
	}
	var data MailAuthResults
	if err := json.Unmarshal([]byte(d.AuthResults.String), &data); err != nil || data.SPF.Result == "" {
		return nil
	}
	return &data
}

func (m MailAuthResults) Status() string {
	if m.DMARC.Result == "pass" || (m.DMARC.Result == "none" && (m.SPF.Aligned || m.DKIM.Aligned)) {
		return "pass"
	}
	if m.DMARC.Result == "fail" || (m.DMARC.Result == "none" && (m.SPF.Result == "fail" || m.SPF.Result == "softfail" || m.SPF.Result == "pass" || m.DKIM.Result == "fail" || m.DKIM.Result == "pass")) {
		return "fail"
	}
	return "none"
}
