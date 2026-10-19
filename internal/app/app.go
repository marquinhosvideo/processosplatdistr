// Package app é o composition root: config → slog → banco → migrations →
// bootstrap → módulos → servidor HTTP → jobs, na ordem da §10.3 do SDD. Cada
// marco acrescenta a sua etapa aqui.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/marquinhosvideo/processosplatdistr/internal/buildinfo"
	"github.com/marquinhosvideo/processosplatdistr/internal/config"
	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/migrate"
	"github.com/marquinhosvideo/processosplatdistr/internal/server"
	"github.com/marquinhosvideo/processosplatdistr/internal/web"
)

// ShutdownTimeout é o prazo para as requisições em andamento no SIGTERM.
const ShutdownTimeout = 20 * time.Second

// ConnectRetry é quanto a subida espera o banco responder (§10.3).
const ConnectRetry = 30 * time.Second

// NewLogger cria o logger JSON da aplicação e o instala como slog.Default.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

// App guarda o que a subida construiu.
type App struct {
	cfg    *config.Config
	logger *slog.Logger
	runner *migrate.Runner

	DB        *sql.DB
	OrgUnitID int64 // gerência DEFAULT_ORG_UNIT_CODE
}

// Start executa as etapas de banco da §10.3: conecta (com retry), confere a
// versão do MySQL, aplica as migrations (MIGRATE_ON_START) ou confere o
// schema, faz o bootstrap e confere o max_allowed_packet. Qualquer falha
// impede a subida: o deploy falha e o Tsuru mantém as unidades antigas.
func Start(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*App, error) {
	appCfg, migCfg, err := db.FromConfig(cfg)
	if err != nil {
		return nil, err
	}
	runner, err := migrate.New(logger)
	if err != nil {
		return nil, err
	}
	h, err := db.Connect(ctx, appCfg, db.PoolFromConfig(cfg), ConnectRetry, logger)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, logger: logger, runner: runner, DB: h}
	if err := a.start(ctx, migCfg); err != nil {
		h.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) start(ctx context.Context, migCfg *mysql.Config) error {
	version, err := db.CheckVersion(ctx, a.DB)
	if err != nil {
		return err
	}
	a.logger.Info("db.connected", slog.String("server_version", version))

	if a.cfg.MigrateOnStart {
		if err := a.migrate(ctx, migCfg); err != nil {
			return err
		}
	} else {
		st, err := a.runner.Check(ctx, a.DB)
		if err != nil {
			return fmt.Errorf("MIGRATE_ON_START=false: %w", err)
		}
		a.logger.Info("migrate.checked", slog.Int("schema_version", int(st.Current)), slog.Int("binary_latest", int(st.Latest)))
	}

	if a.OrgUnitID, err = ensureOrgUnit(ctx, a.DB, a.cfg.DefaultOrgUnitCode, a.cfg.DefaultOrgUnitName, a.logger); err != nil {
		return err
	}

	if a.cfg.FilesBackend == config.FilesMySQL {
		if err := db.CheckMaxAllowedPacket(ctx, a.DB, a.cfg.FilesMaxBytes+1<<20); err != nil {
			return fmt.Errorf("FILES_BACKEND=mysql: %w", err)
		}
	}
	return nil
}

// migrate roda o migrate.Up na conexão dedicada (MIGRATION_DATABASE_DSN ou o
// DSN da app), sob o GET_LOCK que serializa as unidades.
func (a *App) migrate(ctx context.Context, migCfg *mysql.Config) error {
	mdb, err := migrate.Open(migCfg)
	if err != nil {
		return err
	}
	defer mdb.Close()
	applied, err := a.runner.Up(ctx, mdb)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	a.logger.Info("migrate.done", slog.Int("applied", len(applied)), slog.Int("binary_latest", int(a.runner.Migrations[len(a.runner.Migrations)-1].Version)))
	return nil
}

// Close fecha o pool do banco.
func (a *App) Close() error { return a.DB.Close() }

// Serve atende HTTP em ln até ctx ser cancelado e então encerra com
// graceful shutdown.
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	return a.serve(ctx, ln, web.Dist())
}

// ready é o /readyz: ping no banco e schema compatível com o binário.
func (a *App) ready(ctx context.Context) (uint32, error) {
	if err := a.DB.PingContext(ctx); err != nil {
		return 0, err
	}
	st, err := a.runner.Status(ctx, a.DB)
	if err != nil {
		return 0, err
	}
	return st.Current, st.Problem()
}

func (a *App) serve(ctx context.Context, ln net.Listener, dist fs.FS) error {
	logger := a.logger
	spa, err := web.New(dist)
	if err != nil {
		return err
	}
	if !spa.Built() {
		logger.Warn("web.not_built", slog.String("detail", "frontend não compilado: as páginas respondem 503 até rodar make build"))
	}

	srv := server.HTTPServer(server.New(server.Deps{Config: a.cfg, Logger: logger, SPA: spa, Ready: a.ready}), logger)
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
