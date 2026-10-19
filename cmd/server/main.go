// Command server é o binário único da plataforma (SDD §1.2):
//
//	server serve | migrate [up|status|force N] | jobs run <nome> | config check | version
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	_ "time/tzdata" // APP_TZ funciona sem tzdata na imagem

	"github.com/marquinhosvideo/processosplatdistr/internal/app"
	"github.com/marquinhosvideo/processosplatdistr/internal/buildinfo"
	"github.com/marquinhosvideo/processosplatdistr/internal/config"
	"github.com/marquinhosvideo/processosplatdistr/internal/db"
)

const usage = `Uso: server <comando>

Comandos:
  serve                        sobe o servidor HTTP
  migrate [up|status|force N]  aplica ou inspeciona as migrations do banco
  jobs run <nome>              executa um job em background de forma síncrona
  config check                 valida a configuração do ambiente, incluindo as travas de produção
  version                      mostra a versão do binário
`

// Códigos de saída.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// env isola o processo para os testes.
type env struct {
	getenv func(string) string
	stdout io.Writer
	stderr io.Writer
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], env{getenv: os.Getenv, stdout: os.Stdout, stderr: os.Stderr})
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, e env) int {
	if len(args) == 0 {
		fmt.Fprint(e.stderr, usage)
		return exitUsage
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case "serve":
		if len(rest) != 0 {
			return usageError(e, "serve não recebe argumentos")
		}
		return serve(ctx, e)
	case "version":
		fmt.Fprintln(e.stdout, buildinfo.Version)
		return exitOK
	case "config":
		if len(rest) != 1 || rest[0] != "check" {
			return usageError(e, "use: server config check")
		}
		return configCheck(e)
	case "migrate":
		return migrateCmd(ctx, e, rest)
	case "jobs":
		// O dispatcher já conhece o comando; a implementação chega com o
		// runner de jobs.
		fmt.Fprintf(e.stderr, "server: o comando %s ainda não está disponível nesta versão\n", cmd)
		return exitError
	case "help", "-h", "--help":
		fmt.Fprint(e.stdout, usage)
		return exitOK
	default:
		return usageError(e, fmt.Sprintf("comando desconhecido %q", cmd))
	}
}

func usageError(e env, msg string) int {
	fmt.Fprintf(e.stderr, "server: %s\n\n%s", msg, usage)
	return exitUsage
}

// loadConfig carrega e valida, inclusive o formato dos DSNs do banco; os
// problemas vão para stderr, um por linha.
func loadConfig(e env) (*config.Config, bool) {
	cfg, err := config.FromEnv(e.getenv)
	if err == nil {
		if _, _, err = db.FromConfig(cfg); err == nil {
			return cfg, true
		}
		err = &config.Error{Problems: []string{err.Error()}}
	}
	var ce *config.Error
	if errors.As(err, &ce) {
		fmt.Fprintln(e.stderr, "configuração inválida:")
		for _, p := range ce.Problems {
			fmt.Fprintf(e.stderr, "  - %s\n", p)
		}
	} else {
		fmt.Fprintf(e.stderr, "configuração inválida: %v\n", err)
	}
	return nil, false
}

func configCheck(e env) int {
	cfg, ok := loadConfig(e)
	if !ok {
		return exitError
	}
	fmt.Fprintf(e.stdout, "configuração válida (APP_ENV=%s)\n", cfg.Env)
	return exitOK
}

func serve(ctx context.Context, e env) int {
	cfg, ok := loadConfig(e)
	if !ok {
		return exitError
	}
	logger := app.NewLogger(e.stdout, cfg.LogLevel)
	logger.LogAttrs(ctx, slog.LevelInfo, "server.start", append(cfg.LogAttrs(), slog.String("version", buildinfo.Version))...)

	// Banco, migrations e bootstrap antes de abrir a porta: o healthcheck só
	// responde quando a unidade está pronta (§10.3).
	a, err := app.Start(ctx, cfg, logger)
	if err != nil {
		logger.Error("server.start_failed", slog.Any("err", err))
		return exitError
	}
	defer a.Close()

	ln, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(cfg.Port)))
	if err != nil {
		logger.Error("server.listen_failed", slog.Any("err", err))
		return exitError
	}
	if err := a.Serve(ctx, ln); err != nil {
		logger.Error("server.failed", slog.Any("err", err))
		return exitError
	}
	return exitOK
}
