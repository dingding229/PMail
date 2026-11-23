package send

import (
	"errors"
	"strings"
	"testing"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestExplainDialFailure(t *testing.T) {
	err := explainDialFailure(timeoutErr{}, "25")
	if !strings.Contains(err.Error(), "may block outbound SMTP") || !errors.Is(err, timeoutErr{}) {
		t.Fatalf("port 25 timeout: %v", err)
	}
	if got := explainDialFailure(timeoutErr{}, "587"); got != (timeoutErr{}) {
		t.Fatalf("other ports unchanged: %v", got)
	}
	plain := errors.New("connection refused")
	if got := explainDialFailure(plain, "25"); got != plain {
		t.Fatalf("non-timeout unchanged: %v", got)
	}
}
