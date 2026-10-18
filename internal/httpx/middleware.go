package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

// Headers próprios da aplicação.
const (
	RequestIDHeader  = "X-Request-Id"
	AppVersionHeader = "X-App-Version"
)

// CSPs por classe de resposta (§7.8). "style-src 'unsafe-inline'" no SPA é
// exigência do Mantine.
const (
	CSPSPA      = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	CSPAPI      = "default-src 'none'; frame-ancestors 'none'"
	CSPDownload = "default-src 'none'; sandbox"
	CSPShare    = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

// Middleware decora um http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain aplica os middlewares a h; o primeiro da lista é o mais externo.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

// recorder guarda o status escrito, para o log de acesso e para o Recover
// saber se ainda dá para responder.
type recorder struct {
	http.ResponseWriter
	status int
}

func (rw *recorder) WriteHeader(code int) {
	if rw.status == 0 && code >= 200 {
		rw.status = code
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recorder) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap permite que http.ResponseController alcance o writer original
// (Flush, deadlines).
func (rw *recorder) Unwrap() http.ResponseWriter { return rw.ResponseWriter }

// Recover transforma um panic num 500 problem+json, com stack no log. Se a
// resposta já começou, aborta a conexão para o cliente não receber um corpo
// truncado como se fosse completo.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &recorder{ResponseWriter: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				// O RequestID roda dentro deste middleware; o ID está no header.
				ctx := withRequest(r.Context(), w.Header().Get(RequestIDHeader), logger)
				Logger(ctx).Error("http.panic", slog.String("panic", fmt.Sprint(v)), slog.String("stack", string(debug.Stack())))
				if rw.status != 0 {
					panic(http.ErrAbortHandler)
				}
				writeProblem(rw, r.WithContext(ctx), Internal(nil))
			}()
			next.ServeHTTP(rw, r)
		})
	}
}

// AssignRequestID gera o ID da requisição (16 hex), devolve-o em X-Request-Id
// e põe no contexto um logger com request_id. Um X-Request-Id recebido é
// ignorado: não confiamos em valor do cliente para correlacionar logs.
func AssignRequestID(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newRequestID()
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(withRequest(r.Context(), id, logger)))
		})
	}
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read não falha desde o Go 1.24
	return hex.EncodeToString(b[:])
}

// AppVersion envia X-App-Version em toda resposta; o SPA compara com a
// própria versão para oferecer o recarregamento.
func AppVersion(version string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(AppVersionHeader, version)
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders aplica os headers comuns a todas as respostas e a CSP da
// classe do caminho (SPA, API ou página pública /s). Handlers podem
// sobrescrever a CSP (ex.: download). hsts liga o Strict-Transport-Security,
// só quando APP_BASE_URL é https.
func SecurityHeaders(hsts bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			switch p := r.URL.Path; {
			case p == "/s" || strings.HasPrefix(p, "/s/"):
				h.Set("Content-Security-Policy", CSPShare)
				h.Set("Referrer-Policy", "no-referrer")
				h.Set("X-Robots-Tag", "noindex, nofollow")
				h.Set("Cache-Control", "no-store")
			case isAPIPath(p):
				h.Set("Content-Security-Policy", CSPAPI)
			default:
				h.Set("Content-Security-Policy", CSPSPA)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isAPIPath cobre as respostas que não são documentos do SPA.
func isAPIPath(p string) bool {
	switch p {
	case "/api", "/auth", "/healthz", "/readyz", "/metrics":
		return true
	}
	return strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/auth/")
}

// NoStore marca a resposta como não armazenável (todo /api e /auth).
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// AccessLog registra uma linha por requisição: request_id, method, route,
// status, duration_ms e user_id. A rota é o padrão do ServeMux (r.Pattern),
// nunca o caminho bruto; query string, corpo, cookies e headers não são
// registrados. Precisa ficar logo antes do ServeMux, que preenche r.Pattern
// na requisição que recebe. As rotas em quiet (ex.: o healthcheck do Tsuru)
// só aparecem em nível debug quando dão certo.
func AccessLog(quiet ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			st := &logState{}
			r = r.WithContext(context.WithValue(r.Context(), logStateKey, st))
			rw := &recorder{ResponseWriter: w}
			defer func() {
				v := recover()
				status := rw.status
				switch {
				case status == 0 && v != nil:
					status = http.StatusInternalServerError // o Recover responde 500
				case status == 0:
					status = http.StatusOK // handler sem escrita: o net/http responde 200
				}
				level := slog.LevelInfo
				if status < 400 && slices.Contains(quiet, r.Pattern) {
					level = slog.LevelDebug
				}
				attrs := []slog.Attr{
					slog.String("method", r.Method),
					slog.String("route", r.Pattern),
					slog.Int("status", status),
					slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				}
				if st.userID != 0 {
					attrs = append(attrs, slog.Int64("user_id", st.userID))
				}
				Logger(r.Context()).LogAttrs(r.Context(), level, "http.request", attrs...)
				if v != nil {
					panic(v)
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}
