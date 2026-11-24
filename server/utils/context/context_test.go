package context

import (
	stdctx "context"
	"testing"
	"time"
)

func TestEmptyContextBehavesLikeBackground(t *testing.T) {
	c := &Context{}
	if _, ok := c.Deadline(); ok || c.Err() != nil || c.Done() != nil || c.Value("x") != nil {
		t.Fatal("empty context is not background-like")
	}
	// Deriving a context from it must not panic.
	d, cancel := stdctx.WithTimeout(c, time.Second)
	defer cancel()
	if _, ok := d.Deadline(); !ok {
		t.Fatal("derived deadline missing")
	}
	inner, stop := stdctx.WithCancel(stdctx.Background())
	stop()
	if (&Context{Context: inner}).Err() == nil {
		t.Fatal("inner context not used")
	}
}
