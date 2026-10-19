package migrate_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/marquinhosvideo/processosplatdistr/internal/db/dbtest"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/migrate"
)

func TestMain(m *testing.M) { dbtest.Main(m) }

func open(t *testing.T, cfg *mysql.Config) *sql.DB {
	t.Helper()
	h, err := migrate.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func embedded(t *testing.T, logs *bytes.Buffer) *migrate.Runner {
	t.Helper()
	ms, err := migrate.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	return &migrate.Runner{Migrations: ms, Logger: slog.New(slog.NewJSONHandler(logs, nil))}
}

func mig(version uint32, name, sqlText string) migrate.Migration {
	return migrate.Migration{Version: version, Name: name, SQL: sqlText, Checksum: sha256.Sum256([]byte(sqlText))}
}

func tables(t *testing.T, h *sql.DB) []string {
	t.Helper()
	rows, err := h.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func TestUpOnEmptyDatabaseAndRerun(t *testing.T) {
	cfg := dbtest.NewDatabase(t)
	h := open(t, cfg)
	var logs bytes.Buffer
	r := embedded(t, &logs)

	applied, err := r.Up(t.Context(), h)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(r.Migrations) {
		t.Fatalf("aplicadas %d de %d", len(applied), len(r.Migrations))
	}
	want := []string{"access_log", "app_settings", "audit_log", "job_runs", "job_titles", "notifications",
		"org_units", "people", "schema_migrations", "sessions", "teams", "user_activity_days", "users"}
	if got := tables(t, h); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tabelas = %v", got)
	}
	st, err := r.Status(t.Context(), h)
	if err != nil {
		t.Fatal(err)
	}
	if st.Problem() != nil || st.Current != st.Latest || st.Current == 0 || st.Applied[0].AppliedAt.Time.IsZero() {
		t.Fatalf("status = %+v (%v)", st, st.Problem())
	}
	if !strings.Contains(logs.String(), `"msg":"migrate.applied"`) {
		t.Errorf("sem log de aplicação: %s", logs.String())
	}

	// Reexecução: nada a aplicar, nada muda.
	firstApplied := st.Applied[0].AppliedAt.Time
	again, err := r.Up(t.Context(), h)
	if err != nil || len(again) != 0 {
		t.Fatalf("reexecução aplicou %d: %v", len(again), err)
	}
	st2, err := r.Status(t.Context(), h)
	if err != nil || len(st2.Applied) != len(st.Applied) || !st2.Applied[0].AppliedAt.Time.Equal(firstApplied) {
		t.Errorf("reexecução alterou schema_migrations: %+v %v", st2.Applied, err)
	}

	// O próprio DDL também é reexecutável (CREATE TABLE IF NOT EXISTS).
	if _, err := h.Exec(r.Migrations[0].SQL); err != nil {
		t.Errorf("0001 não é reexecutável: %v", err)
	}
}

func TestChecksumChangedFails(t *testing.T) {
	cfg := dbtest.NewDatabase(t)
	h := open(t, cfg)
	v1 := &migrate.Runner{Migrations: []migrate.Migration{mig(1, "a", "CREATE TABLE IF NOT EXISTS a (id INT PRIMARY KEY);")}}
	if _, err := v1.Up(t.Context(), h); err != nil {
		t.Fatal(err)
	}
	edited := &migrate.Runner{Migrations: []migrate.Migration{
		mig(1, "a", "CREATE TABLE IF NOT EXISTS a (id BIGINT PRIMARY KEY);"),
		mig(2, "b", "CREATE TABLE IF NOT EXISTS b (id INT PRIMARY KEY);"),
	}}
	_, err := edited.Up(t.Context(), h)
	if !errors.Is(err, migrate.ErrChecksum) || !strings.Contains(err.Error(), "0001 (a)") {
		t.Fatalf("err = %v, want ErrChecksum", err)
	}
	if got := tables(t, h); strings.Contains(strings.Join(got, ","), ",b,") || len(got) != 2 {
		t.Errorf("aplicou a pendente apesar do checksum: %v", got)
	}
	if _, err := edited.Check(t.Context(), h); !errors.Is(err, migrate.ErrChecksum) {
		t.Errorf("Check = %v", err)
	}
}

func TestDirtyBlocksUntilForce(t *testing.T) {
	cfg := dbtest.NewDatabase(t)
	h := open(t, cfg)
	good := mig(1, "a", "CREATE TABLE IF NOT EXISTS a (id INT PRIMARY KEY);")
	// O segundo comando falha depois do primeiro ter sido executado: o DDL
	// não volta atrás e a versão fica dirty.
	broken := mig(2, "b", "CREATE TABLE IF NOT EXISTS b (id INT PRIMARY KEY);\nCREATE TABLE c (id INT PRIMARY KEY, x TIPO_INEXISTENTE);")
	r := &migrate.Runner{Migrations: []migrate.Migration{good, broken}}

	applied, err := r.Up(t.Context(), h)
	if !errors.Is(err, migrate.ErrDirty) || !strings.Contains(err.Error(), "server migrate force 2") || len(applied) != 1 {
		t.Fatalf("Up = %d, %v", len(applied), err)
	}
	st, err := r.Status(t.Context(), h)
	if err != nil || st.Dirty == nil || st.Dirty.Version != 2 || st.Current != 1 {
		t.Fatalf("status = %+v %v", st, err)
	}

	// Dirty bloqueia o Up (sem executar nada) e o Check do serve.
	fixed := mig(2, "b", "CREATE TABLE IF NOT EXISTS b (id INT PRIMARY KEY);\nCREATE TABLE IF NOT EXISTS c (id INT PRIMARY KEY);")
	r.Migrations[1] = fixed
	if _, err := r.Up(t.Context(), h); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("Up com dirty = %v", err)
	}
	if got := strings.Join(tables(t, h), ","); strings.Contains(got, ",c,") {
		t.Errorf("Up rodou com dirty: %s", got)
	}
	if _, err := r.Check(t.Context(), h); !errors.Is(err, migrate.ErrDirty) {
		t.Errorf("Check = %v", err)
	}

	// Correção manual + force: limpa com o checksum do arquivo atual.
	if _, err := h.Exec("CREATE TABLE c (id INT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := r.Force(t.Context(), h, 2); err != nil {
		t.Fatal(err)
	}
	if err := r.Force(t.Context(), h, 9); err == nil {
		t.Error("force de versão inexistente aceito")
	}
	st, err = r.Status(t.Context(), h)
	if err != nil || st.Problem() != nil || st.Current != 2 || st.Applied[1].Checksum != fixed.Checksum {
		t.Fatalf("depois do force: %+v %v", st, st.Problem())
	}
	if again, err := r.Up(t.Context(), h); err != nil || len(again) != 0 {
		t.Errorf("Up depois do force = %d %v", len(again), err)
	}
}

func TestConcurrentRunners(t *testing.T) {
	cfg := dbtest.NewDatabase(t)
	const n = 4
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		total int
		errs  []error
	)
	for i := 0; i < n; i++ {
		// Cada "unidade" com a sua conexão dedicada.
		h := open(t, cfg)
		var logs bytes.Buffer
		r := embedded(t, &logs)
		wg.Add(1)
		go func() {
			defer wg.Done()
			applied, err := r.Up(context.Background(), h)
			mu.Lock()
			defer mu.Unlock()
			total += len(applied)
			if err != nil {
				errs = append(errs, err)
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("erros: %v", errs)
	}
	ms, _ := migrate.Embedded()
	if total != len(ms) {
		t.Errorf("aplicações = %d, want %d (cada versão uma vez)", total, len(ms))
	}
	h := open(t, cfg)
	var rows int
	if err := h.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE dirty = 0").Scan(&rows); err != nil || rows != len(ms) {
		t.Errorf("schema_migrations = %d %v", rows, err)
	}
}

func TestLockTimeout(t *testing.T) {
	cfg := dbtest.NewDatabase(t)
	holder := open(t, cfg)
	conn, err := holder.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var got int
	if err := conn.QueryRowContext(t.Context(), "SELECT GET_LOCK(?, 1)", migrate.LockName).Scan(&got); err != nil || got != 1 {
		t.Fatalf("GET_LOCK = %d %v", got, err)
	}
	r := &migrate.Runner{Migrations: []migrate.Migration{mig(1, "a", "CREATE TABLE a (id INT PRIMARY KEY);")}, LockTimeout: time.Second}
	start := time.Now()
	if _, err := r.Up(t.Context(), open(t, cfg)); !errors.Is(err, migrate.ErrLocked) {
		t.Fatalf("Up = %v, want ErrLocked", err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Error("não esperou o lock")
	}
	if _, err := conn.ExecContext(t.Context(), "DO RELEASE_LOCK(?)", migrate.LockName); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Up(t.Context(), open(t, cfg)); err != nil {
		t.Fatalf("Up depois do release: %v", err)
	}
}

func TestSchemaAheadAndBehind(t *testing.T) {
	cfg := dbtest.NewDatabase(t)
	h := open(t, cfg)
	var logs bytes.Buffer
	r := &migrate.Runner{
		Migrations: []migrate.Migration{mig(1, "a", "CREATE TABLE a (id INT PRIMARY KEY);"), mig(2, "b", "CREATE TABLE b (id INT PRIMARY KEY);")},
		Logger:     slog.New(slog.NewJSONHandler(&logs, nil)),
	}

	// Sem a tabela schema_migrations: tudo pendente, schema atrás.
	if _, err := r.Check(t.Context(), h); !errors.Is(err, migrate.ErrBehind) || !strings.Contains(err.Error(), "MIGRATE_ON_START=true") {
		t.Fatalf("Check em banco vazio = %v", err)
	}
	if _, err := r.Up(t.Context(), h); err != nil {
		t.Fatal(err)
	}

	// Um binário anterior (só a 0001) vê o schema à frente: segue com aviso.
	old := &migrate.Runner{Migrations: r.Migrations[:1], Logger: r.Logger}
	st, err := old.Check(t.Context(), h)
	if err != nil || !st.Ahead() || st.Current != 2 {
		t.Fatalf("Check à frente = %+v %v", st, err)
	}
	if applied, err := old.Up(t.Context(), h); err != nil || len(applied) != 0 {
		t.Fatalf("Up à frente = %d %v", len(applied), err)
	}
	if !strings.Contains(logs.String(), `"msg":"migrate.schema_ahead"`) {
		t.Errorf("sem aviso de schema à frente: %s", logs.String())
	}

	// Um binário novo (0003) vê o schema atrás.
	newer := &migrate.Runner{Migrations: append(append([]migrate.Migration{}, r.Migrations...), mig(3, "c", "CREATE TABLE c (id INT PRIMARY KEY);"))}
	if _, err := newer.Check(t.Context(), h); !errors.Is(err, migrate.ErrBehind) || !strings.Contains(err.Error(), "versão 0002") {
		t.Errorf("Check atrás = %v", err)
	}
}

func TestUpConfigUsesDedicatedConnection(t *testing.T) {
	cfg := dbtest.NewDatabase(t)
	applied, err := migrate.UpConfig(t.Context(), cfg, slog.New(slog.DiscardHandler))
	if err != nil || len(applied) == 0 {
		t.Fatalf("UpConfig = %d %v", len(applied), err)
	}
	// O pool da aplicação continua sem multiStatements.
	app := dbtest.Open(t, cfg)
	if _, err := app.Exec("SELECT 1; SELECT 2"); err == nil {
		t.Error("pool da aplicação aceitou múltiplos comandos")
	}
}
