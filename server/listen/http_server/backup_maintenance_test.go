package http_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/session"
	"github.com/alexedwards/scs/v2"
)

func TestBackupMaintenanceBlocksHTTPButPreservesLocalHealth(t *testing.T) {
	old := session.Instance
	session.Instance = scs.New()
	t.Cleanup(func() { session.Instance = old })
	mux := http.NewServeMux()
	router(mux, "1.0.0")
	handler := apiSessions(mux)
	resume, err := maintenance.Default.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer resume()
	for _, tt := range []struct {
		path, from string
		want       int
	}{
		{"/api/email/list", "198.51.100.8:1234", 503},
		{"/api/admin/backups/restore-status", "198.51.100.8:1234", 503},
		{"/api/ping", "198.51.100.8:1234", 503},
		{"/api/ping", "127.0.0.1:1234", 200},
	} {
		r := httptest.NewRequest(http.MethodGet, tt.path, nil)
		r.RemoteAddr = tt.from
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tt.want || tt.want == 503 && !strings.Contains(w.Body.String(), `"code":"maintenance"`) {
			t.Fatalf("%s from %s: %d %s", tt.path, tt.from, w.Code, w.Body.String())
		}
	}
}
