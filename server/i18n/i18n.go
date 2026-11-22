// Package i18n translates outward-facing PMail messages. Diagnostics in logs
// and actual data from remote services are not translation keys.
package i18n

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Jinnrry/pmail/config"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	"golang.org/x/text/language"
)

const (
	Chinese = "zh-CN"
	English = "en"
)

// Preference accepts common variants of the supported languages. An empty
// preference means the user's language is not explicitly set.
func Preference(value string) (string, bool) {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if value == "" {
		return "", true
	}
	if value == "zhcn" { // historical locale spelling
		return Chinese, true
	}
	tag, err := language.Parse(value)
	if err != nil {
		return "", false
	}
	base, _ := tag.Base()
	switch base.String() {
	case "zh":
		return Chinese, true
	case "en":
		return English, true
	}
	return "", false
}

func normalize(value string) string {
	if lang, _ := Preference(value); lang == Chinese {
		return Chinese
	}
	return English
}

func Default() string { return normalize(config.DefaultLang()) }

func Resolve(r *http.Request) string {
	if explicit := strings.TrimSpace(r.Header.Get("X-PMail-Lang")); explicit != "" {
		return normalize(explicit)
	}
	if header := strings.Join(r.Header.Values("Accept-Language"), ","); strings.TrimSpace(header) != "" {
		tags, _, err := language.ParseAcceptLanguage(header)
		if err == nil && len(tags) != 0 {
			return normalize(tags[0].String())
		}
		return English
	}
	return Default()
}

func ForRequest(r *http.Request) *pcontext.Context {
	return &pcontext.Context{Context: r.Context(), Lang: Resolve(r)}
}

// T uses the selected language in the PMail context. A missing language (e.g.
// for a background system email) falls back to PMAIL_DEFAULT_LANG.
func T(ctx *pcontext.Context, key string, args ...any) string {
	lang := ""
	if ctx != nil {
		lang = ctx.Lang
	}
	return Text(lang, key, args...)
}

func Text(lang, key string, args ...any) string {
	if strings.TrimSpace(lang) == "" {
		lang = Default()
	} else {
		lang = normalize(lang)
	}
	catalog := en
	if lang == Chinese {
		catalog = cn
	}
	format, ok := catalog[key]
	if !ok {
		format = catalog["internal_error"]
	}
	if len(args) != 0 {
		return fmt.Sprintf(format, args...)
	}
	return format
}

// Diagnostic wraps a verbatim third-party error or existing persisted detail.
// Only the PMail explanation is translated; empty diagnostic fields stay empty.
func Diagnostic(ctx *pcontext.Context, key, detail string) string {
	if strings.TrimSpace(detail) == "" {
		return ""
	}
	return T(ctx, key, detail)
}

// Delivery translates stable PMail diagnostics and leaves an external SMTP or
// resolver response intact under the localized label.
func Delivery(ctx *pcontext.Context, key, detail string) string {
	switch strings.TrimSpace(detail) {
	case "delivery result pending; outcome unknown after interruption":
		return T(ctx, "delivery_pending")
	case "local delivery handler is unavailable", "external delivery handler is unavailable":
		return T(ctx, "delivery_unavailable")
	case "mailbox quota exceeded", "mailbox storage quota exceeded":
		return T(ctx, "quota_exceeded")
	}
	if recipients, ok := strings.CutPrefix(detail, "以下收件人投递失败："); ok {
		return T(ctx, "delivery_recipients_failed", recipients)
	}
	return Diagnostic(ctx, key, detail)
}
