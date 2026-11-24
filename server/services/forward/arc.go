package forward

import (
	"errors"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/arc"
	"github.com/Jinnrry/pmail/services/domain"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

// arcResolver is replaced in tests.
var arcResolver arc.KeyResolver

// arcSeal adds an ARC set signed with the forwarding domain's DKIM key, so the
// receiver can trust the SPF/DKIM/DMARC results PMail saw before SRS changed
// the envelope. Set PMAIL_ARC_DISABLE=1 to turn it off. Any failure keeps the
// message unchanged: forwarding never depends on ARC.
func arcSeal(ctx *pcontext.Context, raw []byte, email *parsemail.Email, fromDomain string) []byte {
	if config.Flag("PMAIL_ARC_DISABLE") {
		return raw
	}
	d, key, found := domain.Signer(fromDomain)
	if !found || key == nil || d.DkimSelector == "" {
		return raw
	}
	s := arc.Signer{Domain: d.Name, Selector: d.DkimSelector, Key: key, AuthServID: d.Name, Resolver: arcResolver}
	out, err := s.Seal(ctx, raw, arcResults(email))
	if err != nil {
		if errors.Is(err, arc.ErrUnsupportedKey) {
			return raw // Ed25519 DKIM keys cannot sign ARC; DKIM still applies.
		}
		if errors.Is(err, arc.ErrChainFailed) {
			log.WithContext(ctx).Infof("audit_event=arc_skipped reason=existing_chain_invalid")
		} else {
			log.WithContext(ctx).Warnf("audit_event=arc_failed error=%v", err)
		}
		return raw
	}
	return out
}

// arcResults renders the inbound authentication results PMail recorded.
func arcResults(email *parsemail.Email) string {
	if email == nil || email.AuthResults == nil {
		return "none"
	}
	r := email.AuthResults
	part := func(method string, c models.MailAuthCheck, prop string) string {
		result := strings.ToLower(strings.TrimSpace(c.Result))
		if !validResult(result) {
			result = "none"
		}
		s := method + "=" + result
		if c.Domain != "" && prop != "" && safeToken(c.Domain) {
			s += " " + prop + "=" + c.Domain
		}
		return s
	}
	return strings.Join([]string{
		part("spf", r.SPF, "smtp.mailfrom"),
		part("dkim", r.DKIM, "header.d"),
		part("dmarc", r.DMARC, "header.from"),
	}, ";\r\n\t")
}

func validResult(r string) bool {
	switch r {
	case "pass", "fail", "softfail", "neutral", "none", "temperror", "permerror", "policy":
		return true
	}
	return false
}

func safeToken(v string) bool {
	return v != "" && !strings.ContainsAny(v, " \t\r\n;()\"\\")
}
