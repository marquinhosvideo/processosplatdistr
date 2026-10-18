package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/marquinhosvideo/processosplatdistr/internal/buildinfo"
	"github.com/marquinhosvideo/processosplatdistr/internal/config"
	"github.com/marquinhosvideo/processosplatdistr/internal/httpx"
)

func testConfig(t *testing.T, kv ...string) *config.Config {
	t.Helper()
	env := map[string]string{
		"APP_ENV":               "test",
		"APP_BASE_URL":          "http://localhost:8080",
		"COOKIE_SECURE":         "false",
		"OIDC_ISSUER_URL":       "http://localhost:9000",
		"OIDC_CLIENT_ID":        "processos-dev",
		"ROLE_GROUPS_MANAGER":   "gestores",
		"SESSION_ENC_KEY":       base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"DEFAULT_ORG_UNIT_CODE": "PLATDIST",
		"DEFAULT_ORG_UNIT_NAME": "Plataformas de Distribuição",
	}
	for i := 0; i+1 < len(kv); i += 2 {
		env[kv[i]] = kv[i+1]
	}
	cfg, err := config.FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func testHandler(t *testing.T, logs *bytes.Buffer, kv ...string) http.Handler {
	t.Helper()
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return New(Deps{Config: testConfig(t, kv...), Logger: logger, SPA: spa})
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Fatalf("Content-Type = %q (corpo %s)", ct, rec.Body)
	}
	var body struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID != rec.Header().Get(httpx.RequestIDHeader) {
		t.Errorf("requestId %q difere do header %q", body.RequestID, rec.Header().Get(httpx.RequestIDHeader))
	}
	return body.Code
}

func TestHealthz(t *testing.T) {
	var logs bytes.Buffer
	h := testHandler(t, &logs)
	rec := do(h, "GET", "/healthz")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != buildinfo.Version || len(body) != 2 {
		t.Errorf("corpo = %v", body)
	}
	hdr := rec.Header()
	if hdr.Get("Content-Type") != "application/json; charset=utf-8" || hdr.Get("Cache-Control") != "no-store" ||
		hdr.Get("Content-Security-Policy") != httpx.CSPAPI || hdr.Get(httpx.AppVersionHeader) != buildinfo.Version ||
		len(hdr.Get(httpx.RequestIDHeader)) != 16 || hdr.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers = %v", hdr)
	}
	if rec := do(h, "HEAD", "/healthz"); rec.Code != 200 || rec.Body.Len() != 0 {
		t.Errorf("HEAD: %d %q", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), `"route":"GET /healthz"`) || !strings.Contains(logs.String(), `"level":"DEBUG","msg":"http.request"`) {
		t.Errorf("log do healthcheck: %s", logs.String())
	}
}

func TestUnknownAPIIsJSON404(t *testing.T) {
	var logs bytes.Buffer
	h := testHandler(t, &logs)
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"} {
		for _, p := range []string{"/api/v1/nao-existe", "/api/", "/api"} {
			rec := do(h, m, p)
			if rec.Code != 404 || problemCode(t, rec) != httpx.CodeNotFound {
				t.Errorf("%s %s: %d %s", m, p, rec.Code, rec.Body)
			}
			if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Security-Policy") != httpx.CSPAPI {
				t.Errorf("%s %s: headers = %v", m, p, rec.Header())
			}
		}
	}
}

func TestHSTSOnlyWithHTTPS(t *testing.T) {
	var logs bytes.Buffer
	if got := do(testHandler(t, &logs), "GET", "/healthz").Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS com http: %q", got)
	}
	h := testHandler(t, &logs, "APP_BASE_URL", "https://processos.exemplo.com.br", "COOKIE_SECURE", "true")
	if got := do(h, "GET", "/").Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Errorf("HSTS com https: %q", got)
	}
}

func TestAPIFallbackMethodNotAllowed(t *testing.T) {
	rt := newRouter()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	rt.handle("GET /api/v1/things", ok)
	rt.handle("PUT /api/v1/things/{id}", ok)
	rt.handle("DELETE /api/v1/things/{id}", ok)
	rt.handle(apiFallbackPattern, apiFallback(rt.mux))
	rt.handle(spaPattern, http.NotFoundHandler())

	tests := []struct {
		method, path string
		status       int
		allow        string
	}{
		{"GET", "/api/v1/things", 204, ""},
		{"HEAD", "/api/v1/things", 204, ""},
		{"POST", "/api/v1/things", 405, "GET, HEAD"},
		{"OPTIONS", "/api/v1/things", 405, "GET, HEAD"},
		{"POST", "/api/v1/things/7", 405, "DELETE, PUT"},
		{"GET", "/api/v1/things/7", 405, "DELETE, PUT"},
		{"POST", "/api/v1/other", 404, ""},
		{"OPTIONS", "/api/v1/other", 404, ""},
	}
	for _, tt := range tests {
		rec := do(rt.mux, tt.method, tt.path)
		if rec.Code != tt.status || rec.Header().Get("Allow") != tt.allow {
			t.Errorf("%s %s: %d allow=%q, want %d %q", tt.method, tt.path, rec.Code, rec.Header().Get("Allow"), tt.status, tt.allow)
		}
	}
}

func TestPublicRoutesAllowlist(t *testing.T) {
	expectPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: sem panic", name)
			}
		}()
		f()
	}
	rt := newRouter()
	expectPanic("rota pública fora da allowlist", func() { rt.public("GET /api/v1/me", http.NotFoundHandler()) })
	rt.public("GET /healthz", http.NotFoundHandler())
	expectPanic("rota duplicada", func() { rt.public("GET /healthz", http.NotFoundHandler()) })
}

// Toda rota registrada é pública da allowlist ou um dos fallbacks. Quando as
// rotas autenticadas chegarem, elas entram por um registro com RoleSet.
func TestEveryRegisteredRouteIsKnown(t *testing.T) {
	rt := newRouter()
	routes(rt, Deps{Config: testConfig(t), Logger: slog.Default(), SPA: http.NotFoundHandler()})
	for _, p := range rt.patterns {
		if !slices.Contains(publicRoutes, p) && !isFallback(p) {
			t.Errorf("rota %q sem portão", p)
		}
	}
	for _, want := range []string{"GET /healthz", apiRootPattern, apiFallbackPattern, spaPattern} {
		if !slices.Contains(rt.patterns, want) {
			t.Errorf("rota %q ausente", want)
		}
	}
}

func TestPanicBecomesProblem(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := New(Deps{Config: testConfig(t), Logger: logger, SPA: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("falha") })})
	rec := do(h, "GET", "/qualquer")
	if rec.Code != 500 || problemCode(t, rec) != httpx.CodeInternal {
		t.Errorf("status = %d", rec.Code)
	}
	if !strings.Contains(logs.String(), `"msg":"http.panic"`) || !strings.Contains(logs.String(), `"status":500`) {
		t.Errorf("logs = %s", logs.String())
	}
}

func TestHTTPServerTimeouts(t *testing.T) {
	srv := HTTPServer(http.NotFoundHandler(), slog.Default())
	if srv.ReadHeaderTimeout != ReadHeaderTimeout || srv.ReadTimeout != ReadTimeout || srv.WriteTimeout != WriteTimeout ||
		srv.IdleTimeout != IdleTimeout || srv.MaxHeaderBytes != 64*1024 || srv.ErrorLog == nil {
		t.Errorf("server = %+v", srv)
	}
}
