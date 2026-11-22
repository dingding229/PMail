package email

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/utils/context"
)

func TestC7bComposeJSONLimitAndRecipientBudget(t *testing.T) {
	t.Setenv("PMAIL_MAIL_JSON_MAX_BYTES", "1024")
	for path, handler := range map[string]func(*context.Context, http.ResponseWriter, *http.Request){"send": Send, "draft": SaveDraft, "forward": EmailForward} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/email/"+path, strings.NewReader(`{"text":"`+strings.Repeat("x", 1025)+`"}`))
		handler(&context.Context{}, w, req)
		if w.Code != 413 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	t.Setenv("PMAIL_MAIL_JSON_MAX_BYTES", "65536")
	w := httptest.NewRecorder()
	body := `{"to":[` + strings.Repeat(`{"email":"alice@example.test"},`, 50) + `{"email":"bob@example.test"}]}`
	Send(&context.Context{}, w, httptest.NewRequest("POST", "/api/email/send", strings.NewReader(body)))
	if w.Code != 413 {
		t.Fatalf("recipients: %d %s", w.Code, w.Body.String())
	}
}
