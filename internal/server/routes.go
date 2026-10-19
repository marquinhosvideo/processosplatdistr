package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/buildinfo"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/migrate"
	"github.com/marquinhosvideo/processosplatdistr/internal/httpx"
)

// publicRoutes é a allowlist fixa de rotas sem sessão (§4.2, papel P). Uma
// rota pública fora dela dá panic no registro; as rotas autenticadas entram
// pelo router com RoleSet obrigatório.
var publicRoutes = []string{
	"GET /healthz",
	"GET /readyz",
	"GET /metrics",
	"GET /auth/login",
	"GET /auth/callback",
	"GET /auth/denied",
	"GET /auth/logged-out",
	"GET /s/onboarding",
	"POST /s/api/onboarding",
	"POST /s/api/onboarding/export.md",
}

// Padrões de fallback: /api desconhecido (404/405 em JSON) e o SPA. "/api"
// exato também é registrado para o ServeMux não redirecioná-lo para "/api/".
const (
	apiRootPattern     = "/api"
	apiFallbackPattern = "/api/"
	spaPattern         = "/"
)

func isFallback(pattern string) bool {
	return pattern == apiRootPattern || pattern == apiFallbackPattern || pattern == spaPattern
}

// router é a tabela única de rotas do servidor.
type router struct {
	mux      *http.ServeMux
	patterns []string
}

func newRouter() *router { return &router{mux: http.NewServeMux()} }

func (rt *router) handle(pattern string, h http.Handler) {
	if slices.Contains(rt.patterns, pattern) {
		panic(fmt.Sprintf("server: rota %q registrada duas vezes", pattern))
	}
	rt.patterns = append(rt.patterns, pattern)
	rt.mux.Handle(pattern, h)
}

// public registra uma rota sem sessão; só aceita padrões da allowlist.
func (rt *router) public(pattern string, h http.Handler) {
	if !slices.Contains(publicRoutes, pattern) {
		panic(fmt.Sprintf("server: rota pública %q fora da allowlist", pattern))
	}
	rt.handle(pattern, h)
}

// routes monta a tabela. Novas rotas entram aqui, sempre pelo router.
func routes(rt *router, d Deps) {
	rt.public("GET /healthz", httpx.NoStore(httpx.HandlerFunc(healthz)))
	rt.public("GET /readyz", httpx.NoStore(readyz(d.Ready)))

	api := httpx.NoStore(apiFallback(rt.mux))
	rt.handle(apiRootPattern, api)
	rt.handle(apiFallbackPattern, api)
	rt.handle(spaPattern, d.SPA)
}

// healthz é o liveness do Tsuru: não toca o banco, para que uma oscilação do
// DBaaS não derrube todas as unidades.
func healthz(w http.ResponseWriter, r *http.Request) error {
	httpx.WriteJSON(w, r, http.StatusOK, map[string]string{"status": "ok", "version": buildinfo.Version})
	return nil
}

// ReadyTimeout limita o ping e a leitura do schema no /readyz.
const ReadyTimeout = time.Second

// readyz é a prontidão para monitoração e E2E: banco respondendo em até 1 s
// e schema compatível com o binário. Falha vira 503 sem a causa, que só vai
// para o log.
func readyz(ready func(context.Context) (uint32, error)) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		if ready == nil {
			return httpx.E(httpx.CodeUnavailable).WithDetail("Banco de dados não configurado.")
		}
		ctx, cancel := context.WithTimeout(r.Context(), ReadyTimeout)
		defer cancel()
		v, err := ready(ctx)
		switch {
		case migrate.IsSchemaProblem(err):
			return httpx.E(httpx.CodeUnavailable).WithCause(err).With("schemaVersion", v).
				WithDetail("O schema do banco não é compatível com esta versão da aplicação.")
		case err != nil:
			return httpx.E(httpx.CodeUnavailable).WithCause(err).WithDetail("Banco de dados indisponível.")
		}
		httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"status": "ok", "version": buildinfo.Version, "schemaVersion": v})
		return nil
	})
}

// Métodos testados para decidir entre 404 e 405.
var apiMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// apiFallback responde o que nenhuma rota /api casou: 405 com Allow quando o
// caminho existe com outro método (inclui o preflight OPTIONS, já que não há
// CORS) e 404 caso contrário, sempre em problem+json.
func apiFallback(mux *http.ServeMux) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		var allow []string
		for _, m := range apiMethods {
			if m == r.Method {
				continue
			}
			probe := *r
			probe.Method = m
			if _, pattern := mux.Handler(&probe); pattern != "" && !isFallback(pattern) {
				allow = append(allow, m)
				if m == http.MethodGet {
					allow = append(allow, http.MethodHead)
				}
			}
		}
		if len(allow) == 0 {
			return httpx.NotFound()
		}
		sort.Strings(allow)
		w.Header().Set("Allow", strings.Join(allow, ", "))
		return httpx.E(httpx.CodeMethodNotAllowed)
	})
}
