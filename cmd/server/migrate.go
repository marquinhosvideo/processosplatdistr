package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/migrate"
)

const migrateUsage = "use: server migrate [up|status|force N]"

// migrateCmd: up (padrão) aplica as pendentes; status lista a situação de
// cada versão; force N marca N como aplicada e limpa depois de uma correção
// manual (§3.2). Usa MIGRATION_DATABASE_DSN, ou o DSN da aplicação.
func migrateCmd(ctx context.Context, e env, args []string) int {
	sub := "up"
	if len(args) > 0 {
		sub = args[0]
	}
	var force uint32
	switch {
	case (sub == "up" || sub == "status") && len(args) <= 1:
	case sub == "force" && len(args) == 2:
		n, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil || n == 0 {
			return usageError(e, fmt.Sprintf("versão inválida %q; %s", args[1], migrateUsage))
		}
		force = uint32(n)
	default:
		return usageError(e, migrateUsage)
	}

	cfg, ok := loadConfig(e)
	if !ok {
		return exitError
	}
	// Logs JSON no stderr; o stdout fica com o relatório para a pessoa.
	logger := slog.New(slog.NewJSONHandler(e.stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	_, migCfg, err := db.FromConfig(cfg)
	if err != nil {
		fmt.Fprintf(e.stderr, "server migrate: %v\n", err)
		return exitError
	}
	runner, err := migrate.New(logger)
	if err != nil {
		fmt.Fprintf(e.stderr, "server migrate: %v\n", err)
		return exitError
	}
	h, err := migrate.Open(migCfg)
	if err != nil {
		fmt.Fprintf(e.stderr, "server migrate: %v\n", err)
		return exitError
	}
	defer h.Close()
	if _, err := db.CheckVersion(ctx, h); err != nil {
		fmt.Fprintf(e.stderr, "server migrate: %v\n", err)
		return exitError
	}

	switch sub {
	case "up":
		applied, err := runner.Up(ctx, h)
		for _, m := range applied {
			fmt.Fprintf(e.stdout, "aplicada %s\n", m.File())
		}
		if err != nil {
			fmt.Fprintf(e.stderr, "server migrate: %v\n", err)
			return exitError
		}
		if len(applied) == 0 {
			fmt.Fprintln(e.stdout, "nada a aplicar: schema em dia")
		}
		return exitOK
	case "force":
		if err := runner.Force(ctx, h, force); err != nil {
			fmt.Fprintf(e.stderr, "server migrate: %v\n", err)
			return exitError
		}
		fmt.Fprintf(e.stdout, "versão %04d marcada como aplicada e limpa\n", force)
		return exitOK
	default:
		st, err := runner.Status(ctx, h)
		if err != nil {
			fmt.Fprintf(e.stderr, "server migrate: %v\n", err)
			return exitError
		}
		printStatus(e, runner, st)
		return exitOK
	}
}

func printStatus(e env, r *migrate.Runner, st migrate.Status) {
	applied := map[uint32]migrate.Applied{}
	for _, a := range st.Applied {
		applied[a.Version] = a
	}
	tw := tabwriter.NewWriter(e.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSÃO\tNOME\tSITUAÇÃO\tAPLICADA EM (UTC)")
	row := func(v uint32, name, state string, at string) {
		fmt.Fprintf(tw, "%04d\t%s\t%s\t%s\n", v, name, state, at)
	}
	for _, m := range r.Migrations {
		a, ok := applied[m.Version]
		switch {
		case !ok:
			row(m.Version, m.Name, "pendente", "-")
		case a.Dirty:
			row(m.Version, m.Name, "DIRTY", "-")
		case a.Checksum != m.Checksum:
			row(m.Version, m.Name, "CHECKSUM DIVERGENTE", appliedAt(a))
		default:
			row(m.Version, m.Name, "aplicada", appliedAt(a))
		}
	}
	for _, a := range st.Unknown {
		state := "desconhecida (schema à frente)"
		if a.Dirty {
			state = "DIRTY, desconhecida"
		}
		row(a.Version, a.Name, state, appliedAt(a))
	}
	tw.Flush()
	if err := st.Problem(); err != nil {
		fmt.Fprintf(e.stdout, "\nsituação: %v\n", err)
		return
	}
	fmt.Fprintf(e.stdout, "\nsituação: schema na versão %04d, binário na %04d\n", st.Current, st.Latest)
}

func appliedAt(a migrate.Applied) string {
	if !a.AppliedAt.Valid {
		return "-"
	}
	return a.AppliedAt.Time.UTC().Format(time.DateTime)
}
