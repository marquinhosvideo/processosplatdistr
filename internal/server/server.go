// Package server monta o http.Handler da aplicação: a cadeia de middlewares
// (SDD §1.2) e a tabela única de rotas.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/buildinfo"
	"github.com/marquinhosvideo/processosplatdistr/internal/config"
	"github.com/marquinhosvideo/processosplatdistr/internal/httpx"
)

// Deps são as dependências já construídas pelo composition root (internal/app).
type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	SPA    http.Handler
}

// New devolve o handler raiz:
//
//	recover → requestID → appVersion → securityHeaders → accessLog → mux
//
// O clientIP (TRUSTED_PROXY_CIDRS) entra antes do accessLog junto com a
// autenticação. O accessLog fica colado no ServeMux para ler r.Pattern.
func New(d Deps) http.Handler {
	rt := newRouter()
	routes(rt, d)
	return httpx.Chain(rt.mux,
		httpx.Recover(d.Logger),
		httpx.AssignRequestID(d.Logger),
		httpx.AppVersion(buildinfo.Version),
		httpx.SecurityHeaders(d.Config.HTTPS()),
		httpx.AccessLog("GET /healthz"),
	)
}

// Timeouts do http.Server (§10.3).
const (
	ReadHeaderTimeout = 10 * time.Second
	ReadTimeout       = 60 * time.Second
	WriteTimeout      = 60 * time.Second
	IdleTimeout       = 120 * time.Second
	MaxHeaderBytes    = 64 << 10
)

// HTTPServer devolve o http.Server configurado; quem chama decide o listener.
func HTTPServer(h http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: ReadHeaderTimeout,
		ReadTimeout:       ReadTimeout,
		WriteTimeout:      WriteTimeout,
		IdleTimeout:       IdleTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}
