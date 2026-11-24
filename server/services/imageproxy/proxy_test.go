package imageproxy

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/utils/safenet"
)

func pngBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.Black)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestNormalize(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://img.example/a.png":     true,
		"http://img.example/a.png?x=1":  true,
		"//img.example/a.png":           true,
		"ftp://img.example/a.png":       false,
		"javascript:alert(1)":           false,
		"/api/email/detail":             false,
		"https://user:pw@img.example/x": false,
		"https:///nohost":               false,
	} {
		if _, err := Normalize(raw); (err == nil) != ok {
			t.Errorf("%s: %v", raw, err)
		}
	}
}

func TestFetchRefusesPrivateNetwork(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	_, err := New().Fetch(t.Context(), srv.URL+"/a.png")
	if !errors.Is(err, safenet.ErrBlocked) || hit {
		t.Fatalf("loopback fetched: %v hit=%v", err, hit)
	}
	for _, u := range []string{"http://169.254.169.254/latest", "http://10.0.0.1/", "http://[::1]/"} {
		if _, err := New().Fetch(t.Context(), u); !errors.Is(err, safenet.ErrBlocked) {
			t.Errorf("%s: %v", u, err)
		}
	}
}

func TestFetchSendsNoReaderDetailsAndChecksType(t *testing.T) {
	pic := pngBytes(t)
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		switch r.URL.Path {
		case "/ok.png":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(pic)
		case "/fake.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("<html><script>alert(1)</script></html>"))
		case "/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
		case "/big":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(append(pic, make([]byte, MaxBytes)...))
		case "/redirect":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	f := New()
	f.Client.Transport = srv.Client().Transport // the test server lives on loopback
	img, err := f.Fetch(t.Context(), srv.URL+"/ok.png")
	if err != nil || img.ContentType != "image/png" || !bytes.Equal(img.Body, pic) {
		t.Fatalf("png: %v", err)
	}
	if got.Header.Get("Cookie") != "" || got.Header.Get("Referer") != "" || got.Header.Get("X-Forwarded-For") != "" || !strings.HasPrefix(got.UserAgent(), "Mozilla/5.0") {
		t.Fatalf("request leaks reader details: %v", got.Header)
	}
	if img, err := f.Fetch(t.Context(), srv.URL+"/logo.svg"); err != nil || img.ContentType != "image/svg+xml" {
		t.Fatalf("svg: %v", err)
	}
	for path, want := range map[string]error{"/fake.png": ErrType, "/big": ErrTooLarge, "/missing": ErrUpstream, "/redirect": ErrUpstream} {
		if _, err := f.Fetch(t.Context(), srv.URL+path); !errors.Is(err, want) {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func TestToken(t *testing.T) {
	prev := config.Instance
	t.Cleanup(func() { config.Instance = prev })
	config.Instance = &config.Config{SecretKey: "test-secret"}
	tok := Token(3, 42)
	if tok == "" || !ValidToken(3, 42, tok) {
		t.Fatal("own token rejected")
	}
	if ValidToken(4, 42, tok) || ValidToken(3, 43, tok) || ValidToken(3, 42, "") {
		t.Fatal("token not bound to user and mail")
	}
	config.Instance = &config.Config{}
	if Token(3, 42) != "" || ValidToken(3, 42, "") {
		t.Fatal("works without a secret")
	}
}
