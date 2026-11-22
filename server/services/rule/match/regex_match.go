package match

import (
	"errors"
	"regexp"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

const MaxPatternBytes = 1024

var ErrInvalidPattern = errors.New("rule pattern is too long or is not a Go regular expression")

func Compile(pattern string) (*regexp.Regexp, error) {
	if len(pattern) > MaxPatternBytes {
		return nil, ErrInvalidPattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, ErrInvalidPattern
	}
	return re, nil
}

type RegexMatch struct{ Rule, Field string }

func NewRegexMatch(field, rule string) *RegexMatch { return &RegexMatch{Rule: rule, Field: field} }
func (r *RegexMatch) Match(ctx *context.Context, email *parsemail.Email) bool {
	if email == nil {
		return false
	}
	re, err := Compile(r.Rule)
	if err != nil {
		log.WithContext(ctx).Warn("ignoring mail rule with incompatible regular expression")
		return false
	}
	return re.MatchString(getFieldContent(r.Field, email))
}
