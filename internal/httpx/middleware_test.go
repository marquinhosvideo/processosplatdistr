package httpx

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// logLines devolve as linhas JSON do buffer com a mensagem dada.
func logLines(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("linha de log não é JSON: %s", sc.Bytes())
		}
		if m["msg"] == msg {
			out = append(out, m)
		}
	}
	return out
}

func TestChainOrder(t *testing.T) {
	var order []string
	mw := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "h") }), mw("a"), mw("b"), mw("c"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if strings.Join(order, ",") != "a,b,c,h" {
		t.Errorf("ordem = %v", order)
	}
}

var hex16 = regexp.MustCompile(`^[0-9a-f]{16}$`)

func TestAssignRequestID(t *testing.T) {
	var logs bytes.Buffer
	var seen []string
	h := AssignRequestID(testLogger(&logs))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, RequestID(r.Context()))
		Logger(r.Context()).Info("dentro")
	}))
	var ids []string
	for range 2 {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(RequestIDHeader, "forjado\ninjeção")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		ids = append(ids, rec.Header().Get(RequestIDHeader))
	}
	for i, id := range ids {
		if !hex16.MatchString(id) || seen[i] != id {
			t.Errorf("id %d: header=%q contexto=%q", i, id, seen[i])
		}
	}
	if ids[0] == ids[1] {
		t.Error("ids repetidos")
	}
	lines := logLines(t, &logs, "dentro")
	if len(lines) != 2 || lines[0]["request_id"] != ids[0] {
		t.Errorf("logger do contexto sem request_id: %v", lines)
	}
	if RequestID(httptest.NewRequest("GET", "/", nil).Context()) != "" {
		t.Error("RequestID fora de requisição deveria ser vazio")
	}
	if Logger(t.Context()) != slog.Default() {
		t.Error("Logger fora de requisição deveria ser o default")
	}
}

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	logger := testLogger(&logs)
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom secreto") }),
		Recover(logger), AssignRequestID(logger))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/x", nil))

	if rec.Code != 500 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decodeProblem(t, rec)
	id := rec.Header().Get(RequestIDHeader)
	if body["code"] != CodeInternal || body["requestId"] != id || id == "" {
		t.Errorf("corpo = %v, header id = %q", body, id)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Error("mensagem do panic vazou na resposta")
	}
	lines := logLines(t, &logs, "http.panic")
	if len(lines) != 1 || lines[0]["request_id"] != id || lines[0]["panic"] != "boom secreto" || !strings.Contains(lines[0]["stack"].(string), "middleware_test.go") {
		t.Errorf("log do panic = %v", lines)
	}
	if n := len(logLines(t, &logs, "http.error")); n != 0 {
		t.Errorf("panic logado %d vezes a mais como http.error", n)
	}
}

func TestRecoverAfterWriteAborts(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(testLogger(&logs))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("parcial"))
		panic("tarde demais")
	}))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recover = %v, want http.ErrAbortHandler", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestRecoverPassesAbortHandler(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(testLogger(&logs))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recover = %v", v)
		}
		if logs.Len() != 0 {
			t.Errorf("ErrAbortHandler não deveria ser logado: %s", logs.String())
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestSecurityHeaders(t *testing.T) {
	common := map[string]string{
		"X-Content-Type-Options":     "nosniff",
		"X-Frame-Options":            "DENY",
		"Permissions-Policy":         "camera=(), microphone=(), geolocation=(), payment=()",
		"Cross-Origin-Opener-Policy": "same-origin",
	}
	tests := []struct {
		path, csp, referrer string
		noStore             bool
	}{
		{"/", CSPSPA, "strict-origin-when-cross-origin", false},
		{"/candidatos/12", CSPSPA, "strict-origin-when-cross-origin", false},
		{"/assets/index-abc.js", CSPSPA, "strict-origin-when-cross-origin", false},
		{"/sobre", CSPSPA, "strict-origin-when-cross-origin", false},
		{"/apis", CSPSPA, "strict-origin-when-cross-origin", false},
		{"/api/v1/me", CSPAPI, "strict-origin-when-cross-origin", false},
		{"/api", CSPAPI, "strict-origin-when-cross-origin", false},
		{"/auth/login", CSPAPI, "strict-origin-when-cross-origin", false},
		{"/healthz", CSPAPI, "strict-origin-when-cross-origin", false},
		{"/readyz", CSPAPI, "strict-origin-when-cross-origin", false},
		{"/metrics", CSPAPI, "strict-origin-when-cross-origin", false},
		{"/s/onboarding", CSPShare, "no-referrer", true},
		{"/s/api/onboarding", CSPShare, "no-referrer", true},
	}
	for _, hsts := range []bool{false, true} {
		h := SecurityHeaders(hsts)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		for _, tt := range tests {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", tt.path, nil))
			got := rec.Header()
			for k, v := range common {
				if got.Get(k) != v {
					t.Errorf("%s: %s = %q, want %q", tt.path, k, got.Get(k), v)
				}
			}
			if got.Get("Content-Security-Policy") != tt.csp {
				t.Errorf("%s: CSP = %q, want %q", tt.path, got.Get("Content-Security-Policy"), tt.csp)
			}
			if got.Get("Referrer-Policy") != tt.referrer {
				t.Errorf("%s: Referrer-Policy = %q", tt.path, got.Get("Referrer-Policy"))
			}
			if tt.noStore != (got.Get("Cache-Control") == "no-store") || tt.noStore != (got.Get("X-Robots-Tag") == "noindex, nofollow") {
				t.Errorf("%s: Cache-Control=%q X-Robots-Tag=%q", tt.path, got.Get("Cache-Control"), got.Get("X-Robots-Tag"))
			}
			if wantHSTS := map[bool]string{true: "max-age=31536000", false: ""}[hsts]; got.Get("Strict-Transport-Security") != wantHSTS {
				t.Errorf("%s hsts=%v: HSTS = %q", tt.path, hsts, got.Get("Strict-Transport-Security"))
			}
			if got.Get("Access-Control-Allow-Origin") != "" {
				t.Errorf("%s: header de CORS presente", tt.path)
			}
		}
	}
}

func TestNoStoreAndAppVersion(t *testing.T) {
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), AppVersion("abc123"), NoStore)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/x", nil))
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get(AppVersionHeader) != "abc123" {
		t.Errorf("headers = %v", rec.Header())
	}
}

func TestAccessLog(t *testing.T) {
	var logs bytes.Buffer
	logger := testLogger(&logs)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/candidates/{id}", func(w http.ResponseWriter, r *http.Request) {
		SetUserID(r.Context(), 42)
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /healthz", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("GET /falha", func(w http.ResponseWriter, r *http.Request) { panic("x") })
	h := Chain(mux, Recover(logger), AssignRequestID(logger), AccessLog("GET /healthz"))

	r := httptest.NewRequest("GET", "/api/v1/candidates/77?q=Fulano+de+Tal&token=abc", nil)
	r.Header.Set("Cookie", "pp_sid=segredo-do-cookie")
	r.Header.Set("Authorization", "Bearer segredo-do-header")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/falha", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/nao-existe", nil))

	lines := logLines(t, &logs, "http.request")
	if len(lines) != 4 {
		t.Fatalf("linhas = %v", lines)
	}
	first := lines[0]
	want := map[string]any{
		"level": "INFO", "method": "GET", "route": "GET /api/v1/candidates/{id}",
		"status": float64(202), "user_id": float64(42), "request_id": rec.Header().Get(RequestIDHeader),
	}
	for k, v := range want {
		if first[k] != v {
			t.Errorf("%s = %v, want %v", k, first[k], v)
		}
	}
	if _, ok := first["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms ausente: %v", first)
	}
	for _, leak := range []string{"Fulano", "token=abc", "/candidates/77", "segredo"} {
		if strings.Contains(logs.String(), leak) {
			t.Errorf("log contém %q: %s", leak, logs.String())
		}
	}

	if lines[1]["level"] != "DEBUG" || lines[1]["route"] != "GET /healthz" || lines[1]["status"] != float64(200) {
		t.Errorf("healthcheck = %v", lines[1])
	}
	if _, ok := lines[1]["user_id"]; ok {
		t.Errorf("user_id sem usuário: %v", lines[1])
	}
	if lines[2]["status"] != float64(500) || lines[2]["route"] != "GET /falha" {
		t.Errorf("panic = %v", lines[2])
	}
	if lines[3]["status"] != float64(404) || lines[3]["route"] != "" {
		t.Errorf("rota inexistente = %v", lines[3])
	}
}
