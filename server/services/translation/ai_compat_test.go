package translation

import (
	"errors"
	"strings"
	"testing"
)

func TestCompletionTextFormats(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"string", `{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`, `{"ok":true}`},
		{"fenced", "{\"choices\":[{\"message\":{\"content\":\"```json\\n{\\\"ok\\\":true}\\n```\"}}]}", `{"ok":true}`},
		{"parts", `{"choices":[{"message":{"content":[{"type":"text","text":"{\"ok\":"},{"type":"text","text":"true}"}]}}]}`, `{"ok":true}`},
	} {
		got, ok := completionText([]byte(tc.body))
		if !ok || got != tc.want {
			t.Errorf("%s: got %q %v", tc.name, got, ok)
		}
	}
	if _, ok := completionText([]byte(`{"choices":[]}`)); ok {
		t.Error("empty choices accepted")
	}
}

func TestStrictJSONAcceptsFenceOnly(t *testing.T) {
	var v struct {
		OK bool `json:"ok"`
	}
	if !strictJSON("```json\n{\"ok\":true}\n```", &v) || !v.OK {
		t.Fatal("fenced JSON rejected")
	}
	if strictJSON("Sure! {\"ok\":true}", &v) || strictJSON(`{"ok":true,"OK":true}`, &v) {
		t.Fatal("loose or duplicate JSON accepted")
	}
}

func TestUpstreamDetailRedacts(t *testing.T) {
	key := "sk-live-abcdefghijklmnop"
	err := upstreamDetail(ErrAuth, 401, []byte(`{"error":{"message":"Invalid token: `+key+` (request id 0123456789abcdef0123456789abcdef)\n"}}`), key)
	var up *UpstreamError
	if !errors.As(err, &up) || !errors.Is(err, ErrAuth) || up.Status != 401 {
		t.Fatalf("got %#v", err)
	}
	if strings.Contains(up.Message, "abcdefghijklmnop") || strings.Contains(up.Message, "0123456789abcdef0123") || strings.Contains(up.Message, "\n") || !strings.HasPrefix(up.Message, "Invalid token") {
		t.Fatalf("message not sanitized: %q", up.Message)
	}
	if got := upstreamDetail(ErrProvider, 502, []byte("<html><body>Bad gateway</body></html>"), ""); got.(*UpstreamError).Message != "" {
		t.Fatal("HTML error page leaked")
	}
	if got := upstreamDetail(ErrConnect, 0, nil, ""); got != ErrConnect {
		t.Fatal("transport error wrapped")
	}
}
