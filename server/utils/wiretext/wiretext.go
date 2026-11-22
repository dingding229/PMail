// Package wiretext validates single-line protocol arguments and mail headers.
package wiretext

import (
	"net/mail"
	"strings"
	"unicode"
)

func Line(s string) bool { return !strings.ContainsAny(s, "\r\n\x00") }

func HeaderValue(s string) bool {
	for _, r := range s {
		if r == '\u2028' || r == '\u2029' || (unicode.IsControl(r) && r != '\t') {
			return false
		}
	}
	return true
}

func HeaderName(s string) bool {
	if s == "" || len(s) > 78 {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 || c == ':' {
			return false
		}
	}
	return true
}

func Address(s string) bool {
	if !HeaderValue(s) || s == "" || len(s) > 320 {
		return false
	}
	p, err := mail.ParseAddress(s)
	return err == nil && p.Name == "" && p.Address == s
}
