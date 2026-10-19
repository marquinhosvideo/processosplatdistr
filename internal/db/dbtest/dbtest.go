// Package dbtest é a infraestrutura dos testes de integração com MySQL real
// (SDD §9). Cada pacote chama Main no TestMain e ganha um banco próprio,
// t_<pacote>_<pid>, já migrado e apagado no fim; os pacotes rodam em paralelo
// (go test -p 4), cada um no seu banco. Sem TEST_MYSQL_DSN (ex.:
// app:app@tcp(127.0.0.1:3306)/), DB e afins pulam o teste.
package dbtest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/migrate"
)

// EnvDSN é a variável com o DSN do servidor de teste, sem banco.
const EnvDSN = "TEST_MYSQL_DSN"

var (
	server  *mysql.Config // TEST_MYSQL_DSN com os parâmetros fixos, sem banco
	pkgCfg  *mysql.Config // banco do pacote
	pkgDB   *sql.DB
	pkgName string
	extra   atomic.Int64
)

// Main cria e migra o banco do pacote, roda os testes e apaga o banco. Uso:
//
//	func TestMain(m *testing.M) { dbtest.Main(m) }
func Main(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	raw := os.Getenv(EnvDSN)
	if raw == "" {
		return m.Run()
	}
	if err := setup(raw); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: %v\n(o MySQL local sobe com make mysql; sem %s os testes de integração são pulados)\n", err, EnvDSN)
		return 1
	}
	code := m.Run()
	pkgDB.Close()
	if err := dropDatabase(pkgCfg.DBName); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: %v\n", err)
	}
	return code
}

func setup(raw string) error {
	cfg, err := db.NewConfig(raw, "")
	if err != nil {
		return fmt.Errorf("%s: %w", EnvDSN, err)
	}
	if cfg.DBName != "" {
		return fmt.Errorf("%s deve apontar para o servidor, sem banco (ex.: app:app@tcp(127.0.0.1:3306)/)", EnvDSN)
	}
	server = cfg
	pkgName = packageName()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := db.Connect(ctx, server, db.Pool{MaxOpen: 2, MaxIdle: 1, MaxLifetime: time.Minute}, 10*time.Second, quietLogger())
	if err != nil {
		return err
	}
	defer admin.Close()
	if _, err := db.CheckVersion(ctx, admin); err != nil {
		return err
	}
	dropStale(ctx, admin)

	name := fmt.Sprintf("t_%s_%d", pkgName, os.Getpid())
	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+name+"`"); err != nil {
		return err
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		return fmt.Errorf("criar o banco %s: %w", name, err)
	}
	pkgCfg = server.Clone()
	pkgCfg.DBName = name
	if _, err := migrate.UpConfig(ctx, pkgCfg, quietLogger()); err != nil {
		_ = dropDatabase(name)
		return fmt.Errorf("migrar %s: %w", name, err)
	}
	pkgDB, err = db.Open(pkgCfg, db.DefaultPool)
	return err
}

// packageName deriva o nome do pacote do binário de teste (migrate.test).
func packageName() string {
	base := strings.TrimSuffix(filepath.Base(os.Args[0]), ".test")
	base = strings.ToLower(base)
	base = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(base, "_")
	if base == "" {
		base = "pkg"
	}
	if len(base) > 30 {
		base = base[:30]
	}
	return base
}

// dropStale apaga bancos de execuções anteriores deste pacote cujo processo
// já morreu (teste interrompido). Só com servidor local: num servidor
// compartilhado, o pid pode ser de outra máquina.
func dropStale(ctx context.Context, admin *sql.DB) {
	if !isLocal(server) {
		return
	}
	prefix := "t_" + pkgName + "_"
	rows, err := admin.QueryContext(ctx, "SHOW DATABASES LIKE ?", strings.ReplaceAll(prefix, "_", `\_`)+"%")
	if err != nil {
		return
	}
	var stale []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		pidStr, _, _ := strings.Cut(strings.TrimPrefix(name, prefix), "_")
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == os.Getpid() || alive(pid) {
			continue
		}
		stale = append(stale, name)
	}
	rows.Close()
	for _, name := range stale {
		_, _ = admin.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+name+"`")
	}
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func isLocal(c *mysql.Config) bool {
	if c.Net == "unix" {
		return true
	}
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func dropDatabase(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := db.Open(server, db.Pool{MaxOpen: 1, MaxIdle: 0, MaxLifetime: time.Minute})
	if err != nil {
		return err
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+name+"`"); err != nil {
		return fmt.Errorf("apagar o banco %s: %w", name, err)
	}
	return nil
}

func quietLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func requireServer(t testing.TB) {
	t.Helper()
	if os.Getenv(EnvDSN) == "" {
		t.Skipf("%s não definido; teste de integração pulado (make mysql && make test-int)", EnvDSN)
	}
	if server == nil {
		t.Fatal("dbtest.Main não foi chamado: o pacote precisa de func TestMain(m *testing.M) { dbtest.Main(m) }")
	}
}

// DB devolve o pool do banco do pacote, já migrado. Pula o teste sem
// TEST_MYSQL_DSN.
func DB(t testing.TB) *sql.DB {
	t.Helper()
	requireServer(t)
	return pkgDB
}

// Config devolve uma cópia da config do banco do pacote, para abrir conexões
// extras (ex.: dois "servidores" concorrentes).
func Config(t testing.TB) *mysql.Config {
	t.Helper()
	requireServer(t)
	return pkgCfg.Clone()
}

// Reset apaga os dados de todas as tabelas do banco do pacote, menos
// schema_migrations. Os testes que gravam chamam no início.
func Reset(t testing.TB) {
	t.Helper()
	requireServer(t)
	ctx := context.Background()
	conn, err := pkgDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tables, err := baseTables(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatal(err)
	}
	// A conexão volta ao pool; não pode ficar sem checagem de FK.
	defer func() { _, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1") }()
	for _, table := range tables {
		if table == "schema_migrations" {
			continue
		}
		if _, err := conn.ExecContext(ctx, "DELETE FROM `"+table+"`"); err != nil {
			t.Fatalf("limpar %s: %v", table, err)
		}
	}
}

func baseTables(ctx context.Context, q db.Q) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT table_name FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// NewDatabase cria um banco vazio, sem migrations, apagado no fim do teste.
// É para os testes do runner e da subida do serve.
func NewDatabase(t testing.TB) *mysql.Config {
	t.Helper()
	requireServer(t)
	name := fmt.Sprintf("t_%s_%d_x%d", pkgName, os.Getpid(), extra.Add(1))
	ctx := context.Background()
	admin, err := db.Open(server, db.Pool{MaxOpen: 1, MaxIdle: 0, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatalf("criar o banco %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := dropDatabase(name); err != nil {
			t.Error(err)
		}
	})
	cfg := server.Clone()
	cfg.DBName = name
	return cfg
}

// Open abre um pool para cfg (ex.: o de NewDatabase), fechado no fim do teste.
func Open(t testing.TB, cfg *mysql.Config) *sql.DB {
	t.Helper()
	h, err := db.Open(cfg, db.DefaultPool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// DSN devolve cfg como DSN em texto (para TEST_* e variáveis do serve).
func DSN(cfg *mysql.Config) string { return cfg.FormatDSN() }
