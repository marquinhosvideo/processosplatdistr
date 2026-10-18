package web

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

const indexHTML = `<!doctype html><html lang="pt-BR"><head><script type="module" src="/assets/index-abc123.js"></script></head><body><div id="root"></div></body></html>`

func builtFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte(indexHTML)},
		"robots.txt":              {Data: []byte("User-agent: *\nDisallow: /\n")},
		"favicon.svg":             {Data: []byte("<svg/>")},
		"assets/index-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/index-def456.css": {Data: []byte("body{}")},
		"assets/font-x.woff2":     {Data: []byte("wOF2")},
	}
}

func newSPA(t *testing.T, fsys fs.FS) *SPA {
	t.Helper()
	s, err := New(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func get(h http.Handler, method, target string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestSPAFallbackToIndex(t *testing.T) {
	s := newSPA(t, builtFS())
	if !s.Built() {
		t.Fatal("Built() = false com index.html")
	}
	for _, p := range []string{"/", "/qualquer", "/candidatos/12", "/nine-box/3/meus-liderados", "/candidatos/", "/index.html", "/sobre", "/apis"} {
		rec := get(s, "GET", p+"?x=1")
		if rec.Code != 200 || rec.Body.String() != indexHTML {
			t.Errorf("%s: status=%d corpo=%.40q", p, rec.Code, rec.Body)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", p, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: Cache-Control = %q", p, cc)
		}
		if rec.Header().Get("ETag") == "" {
			t.Errorf("%s: sem ETag", p)
		}
	}
}

func TestSPAStaticFiles(t *testing.T) {
	s := newSPA(t, builtFS())
	tests := []struct {
		path, ctype, cache, body string
	}{
		{"/assets/index-abc123.js", "text/javascript; charset=utf-8", "public, max-age=31536000, immutable", "console.log(1)"},
		{"/assets/index-def456.css", "text/css; charset=utf-8", "public, max-age=31536000, immutable", "body{}"},
		{"/assets/font-x.woff2", "font/woff2", "public, max-age=31536000, immutable", "wOF2"},
		{"/robots.txt", "text/plain; charset=utf-8", "no-cache", "User-agent: *\nDisallow: /\n"},
		{"/favicon.svg", "image/svg+xml", "no-cache", "<svg/>"},
	}
	for _, tt := range tests {
		rec := get(s, "GET", tt.path)
		h := rec.Header()
		if rec.Code != 200 || rec.Body.String() != tt.body || h.Get("Content-Type") != tt.ctype || h.Get("Cache-Control") != tt.cache {
			t.Errorf("%s: status=%d ct=%q cache=%q corpo=%q", tt.path, rec.Code, h.Get("Content-Type"), h.Get("Cache-Control"), rec.Body)
		}
	}
}

func TestSPAMissingFilesAre404(t *testing.T) {
	s := newSPA(t, builtFS())
	for _, p := range []string{"/assets/index-velho.js", "/assets/", "/assets/sub/x", "/favicon.ico", "/x/y.png"} {
		rec := get(s, "GET", p)
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "<html") {
			t.Errorf("%s: status=%d corpo=%q", p, rec.Code, rec.Body)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: 404 cacheável: %q", p, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestSPAReservedPrefixesAreJSON404(t *testing.T) {
	s := newSPA(t, builtFS())
	for _, p := range []string{"/api", "/api/", "/api/v1/nao-existe", "/auth", "/auth/x", "/s/x"} {
		for _, m := range []string{"GET", "POST", "DELETE"} {
			rec := get(s, m, p)
			var body map[string]any
			if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/problem+json" || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body["code"] != "not_found" {
				t.Errorf("%s %s: status=%d ct=%q corpo=%s", m, p, rec.Code, rec.Header().Get("Content-Type"), rec.Body)
			}
		}
	}
}

func TestSPAMethods(t *testing.T) {
	s := newSPA(t, builtFS())
	rec := get(s, "HEAD", "/candidatos")
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Errorf("HEAD: status=%d corpo=%d bytes", rec.Code, rec.Body.Len())
	}
	for _, m := range []string{"POST", "PUT", "DELETE", "OPTIONS"} {
		rec := get(s, m, "/candidatos")
		if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" || rec.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: status=%d allow=%q", m, rec.Code, rec.Header().Get("Allow"))
		}
	}
}

func TestSPAConditionalGet(t *testing.T) {
	s := newSPA(t, builtFS())
	etag := get(s, "GET", "/").Header().Get("ETag")
	if rec := get(s, "GET", "/outra-pagina", "If-None-Match", etag); rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: status = %d", rec.Code)
	}
	jsTag := get(s, "GET", "/assets/index-abc123.js").Header().Get("ETag")
	if jsTag == etag {
		t.Error("ETags iguais para conteúdos diferentes")
	}
}

func TestSPANotBuilt(t *testing.T) {
	s := newSPA(t, fstest.MapFS{"robots.txt": {Data: []byte("User-agent: *\nDisallow: /\n")}})
	if s.Built() {
		t.Fatal("Built() = true sem index.html")
	}
	rec := get(s, "GET", "/qualquer")
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), "Frontend não compilado") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("status=%d corpo=%q cache=%q", rec.Code, rec.Body, rec.Header().Get("Cache-Control"))
	}
	if rec := get(s, "GET", "/robots.txt"); rec.Code != 200 {
		t.Errorf("robots.txt sem build: status=%d", rec.Code)
	}
	if rec := get(s, "GET", "/api/v1/x"); rec.Code != 404 {
		t.Errorf("/api sem build: status=%d", rec.Code)
	}
}

// O dist embutido sempre tem o robots.txt versionado, mesmo num clone limpo.
func TestEmbeddedDist(t *testing.T) {
	data, err := fs.ReadFile(Dist(), "robots.txt")
	if err != nil {
		t.Fatalf("robots.txt fora do embed: %v", err)
	}
	if !bytes.Contains(data, []byte("Disallow: /")) {
		t.Errorf("robots.txt = %q", data)
	}
	if _, err := New(Dist()); err != nil {
		t.Fatal(err)
	}
}

// O Vite copia web/public/robots.txt por cima do versionado a cada build; os
// dois precisam ser idênticos para o git não acusar diferença.
func TestRobotsMatchesWebPublic(t *testing.T) {
	public, err := os.ReadFile("../../web/public/robots.txt")
	if os.IsNotExist(err) {
		t.Skip("web/public/robots.txt ausente")
	}
	if err != nil {
		t.Fatal(err)
	}
	versioned, err := os.ReadFile("dist/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(public, versioned) {
		t.Errorf("internal/web/dist/robots.txt (%q) difere de web/public/robots.txt (%q)", versioned, public)
	}
}
