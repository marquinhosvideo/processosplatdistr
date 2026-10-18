// Package app é o composition root: config → slog → (banco, migrations,
// bootstrap, módulos) → servidor HTTP → jobs. Cada marco acrescenta a sua
// etapa aqui, na ordem da §10.3 do SDD.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/buildinfo"
	"github.com/marquinhosvideo/processosplatdistr/internal/config"
	"github.com/marquinhosvideo/processosplatdistr/internal/httpx"
	"github.com/marquinhosvideo/processosplatdistr/internal/server"
)

// ShutdownTimeout é o prazo para as requisições em andamento no SIGTERM.
const ShutdownTimeout = 20 * time.Second

// NewLogger cria o logger JSON da aplicação e o instala como slog.Default.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

// Serve atende HTTP em ln até ctx ser cancelado e então encerra com
// graceful shutdown. cfg já deve ter passado por config.Load.
func Serve(ctx context.Context, cfg *config.Config, logger *slog.Logger, ln net.Listener) error {
	// O SPA embutido ainda não existe; páginas respondem 404.
	spa := httpx.HandlerFunc(func(http.ResponseWriter, *http.Request) error { return httpx.NotFound() })

	srv := server.HTTPServer(server.New(server.Deps{Config: cfg, Logger: logger, SPA: spa}), logger)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	logger.Info("server.listening", slog.String("addr", ln.Addr().String()), slog.String("version", buildinfo.Version))

	select {
	case err := <-errc:
		return fmt.Errorf("servidor HTTP: %w", err)
	case <-ctx.Done():
	}

	logger.Info("server.shutdown")
	sctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("encerrar o servidor HTTP: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	logger.Info("server.stopped")
	return nil
}
