package translation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// completionText reads choices[0].message.content from an OpenAI-compatible
// chat completion. Some gateways return content as an array of text parts.
func completionText(data []byte) (string, bool) {
	var res struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &res) != nil || len(res.Choices) == 0 {
		return "", false
	}
	raw := res.Choices[0].Message.Content
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return stripFence(text), true
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return "", false
	}
	var b strings.Builder
	for _, part := range parts {
		if part.Type == "" || part.Type == "text" || part.Type == "output_text" {
			b.WriteString(part.Text)
		}
	}
	return stripFence(b.String()), true
}

var fence = regexp.MustCompile("(?s)^\\s*```[A-Za-z0-9_-]*[ \\t]*\\r?\\n(.*?)\\r?\\n?```\\s*$")

// stripFence removes one Markdown code fence that models often put around JSON.
func stripFence(s string) string {
	if m := fence.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

func aiExchange(ctx context.Context, c *http.Client, endpoint string, body any, headers map[string]string, diagnose bool) ([]byte, int, error) {
	if !diagnose {
		return exchange(ctx, c, http.MethodPost, endpoint, body, headers)
	}
	p, err := json.Marshal(body)
	if err != nil {
		return nil, 0, ErrFormat
	}
	return exchangeResponse(ctx, c, http.MethodPost, endpoint, strings.NewReader(string(p)), "application/json", headers, true)
}

// UpstreamError carries the provider's HTTP status and a short message for
// the owner's connection test. errors.Is still matches the fixed error code.
type UpstreamError struct {
	Err     error
	Status  int
	Message string
}

func (e *UpstreamError) Error() string { return e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }

var secretLike = regexp.MustCompile(`(?i)\b(?:sk|pk|rk|ak|key|token)[-_][A-Za-z0-9_-]{8,}|\bBearer\s+\S+|\b[A-Za-z0-9_-]{32,}\b`)

func upstreamDetail(err error, status int, data []byte, key string) error {
	if status == 0 {
		return err
	}
	msg := ""
	var body struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
		Detail  string          `json:"detail"`
	}
	if json.Unmarshal(data, &body) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		var plain string
		switch {
		case json.Unmarshal(body.Error, &nested) == nil && nested.Message != "":
			msg = nested.Message
		case json.Unmarshal(body.Error, &plain) == nil && plain != "":
			msg = plain
		case body.Message != "":
			msg = body.Message
		default:
			msg = body.Detail
		}
	} else if utf8.Valid(data) && !strings.Contains(strings.ToLower(string(data[:min(len(data), 512)])), "<html") {
		msg = string(data)
	}
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "[redacted]")
	}
	msg = secretLike.ReplaceAllString(msg, "[redacted]")
	msg = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, msg)
	msg = strings.Join(strings.Fields(msg), " ")
	if utf8.RuneCountInString(msg) > 300 {
		msg = string([]rune(msg)[:300]) + "…"
	}
	var target *UpstreamError
	if errors.As(err, &target) {
		return err
	}
	return &UpstreamError{Err: err, Status: status, Message: msg}
}
