package context

import (
	"context"
	"time"
)

const (
	LogID = "LogID"
)

type Context struct {
	context.Context `json:"-"`
	UserID          int
	FactorVersion   int64  `json:"-"`
	AppPasswordID   int    `json:"-"`
	AuthVersion     int64  `json:"-"`
	TokenScope      string `json:"-"`
	UserAccount     string
	UserName        string
	Values          map[string]any
	Lang            string
	IsAdmin         bool
}

func (c *Context) SetValue(key string, value any) {
	if c.Values == nil {
		c.Values = map[string]any{}
	}
	c.Values[key] = value

}

func (c *Context) GetValue(key string) any {
	if c.Values == nil {
		return nil
	}
	return c.Values[key]
}

// A Context built as &Context{} has no standard context inside. Treat it as
// context.Background so passing it to database, DNS or HTTP calls is safe.
func (c Context) base() context.Context {
	if c.Context == nil {
		return context.Background()
	}
	return c.Context
}

func (c Context) Deadline() (time.Time, bool) { return c.base().Deadline() }
func (c Context) Done() <-chan struct{}       { return c.base().Done() }
func (c Context) Err() error                  { return c.base().Err() }
func (c Context) Value(key any) any           { return c.base().Value(key) }
