package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/marquinhosvideo/processosplatdistr/internal/config"
	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/dbtest"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/migrate"
)

func TestMain(m *testing.M) { dbtest.Main(m) }

// syncBuffer evita corrida entre o servidor escrevendo logs e o teste lendo.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func testConfig(t *testing.T, kv ...string) *config.Config {
	t.Helper()
	env := map[string]string{
		"APP_ENV":               "test",
		"APP_BASE_URL":          "http://localhost:8080",
		"COOKIE_SECURE":         "false",
		"OIDC_ISSUER_URL":       "http://localhost:9000",
		"OIDC_CLIENT_ID":        "processos-dev",
		"ROLE_GROUPS_MANAGER":   "gestores",
		"SESSION_ENC_KEY":       base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"DEFAULT_ORG_UNIT_CODE": "PLATDIST",
		"DEFAULT_ORG_UNIT_NAME": "Plataformas de Distribuição",
		"DATABASE_DSN":          "app:app@tcp(127.0.0.1:3306)/app",
	}
	for i := 0; i+1 < len(kv); i += 2 {
		env[kv[i]] = kv[i+1]
	}
	cfg, err := config.FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// offlineApp é um App cujo banco nunca responde (porta fechada), para testar
// o HTTP sem MySQL.
func offlineApp(t *testing.T, logger *slog.Logger) *App {
	t.Helper()
	mc, err := db.NewConfig("app:app@tcp(127.0.0.1:1)/app", "")
	if err != nil {
		t.Fatal(err)
	}
	h, err := db.Open(mc, db.DefaultPool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	runner, err := migrate.New(logger)
	if err != nil {
		t.Fatal(err)
	}
	return &App{cfg: testConfig(t), logger: logger, runner: runner, DB: h}
}

func startServe(t *testing.T, a *App, dist fstest.MapFS) (base string, stop func() error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.serve(ctx, ln, dist) }()
	stop = func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("Serve não encerrou depois do cancelamento")
			return nil
		}
	}
	t.Cleanup(func() { cancel() })
	return "http://" + ln.Addr().String(), stop
}

var client = &http.Client{Timeout: 5 * time.Second}

func fetch(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestServeAndGracefulShutdown(t *testing.T) {
	logs := &syncBuffer{}
	a := offlineApp(t, newLogger(logs))
	base, stop := startServe(t, a, fstest.MapFS{"index.html": {Data: []byte("<!doctype html>spa")}})

	if code, body := fetch(t, base+"/healthz"); code != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("/healthz = %d %s", code, body)
	}
	// Sem banco, o liveness segue 200 e a prontidão cai para 503.
	if code, body := fetch(t, base+"/readyz"); code != 503 || !strings.Contains(body, `"code":"unavailable"`) || !strings.Contains(body, "Banco de dados indisponível") {
		t.Errorf("/readyz sem banco = %d %s", code, body)
	}
	if code, body := fetch(t, base+"/qualquer"); code != 200 || body != "<!doctype html>spa" {
		t.Errorf("/qualquer = %d %s", code, body)
	}
	if code, _ := fetch(t, base+"/api/v1/nada"); code != 404 {
		t.Errorf("/api/v1/nada = %d", code)
	}
	if err := stop(); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	for _, msg := range []string{"server.listening", "server.shutdown", "server.stopped"} {
		if !strings.Contains(logs.String(), `"msg":"`+msg+`"`) {
			t.Errorf("log sem %s: %s", msg, logs.String())
		}
	}
	if strings.Contains(logs.String(), "web.not_built") {
		t.Error("aviso de frontend não compilado com index.html presente")
	}
}

func TestServeWarnsWhenFrontendMissing(t *testing.T) {
	logs := &syncBuffer{}
	a := offlineApp(t, newLogger(logs))
	base, stop := startServe(t, a, fstest.MapFS{"robots.txt": {Data: []byte("User-agent: *\n")}})
	if code, body := fetch(t, base+"/"); code != 503 || !strings.Contains(body, "Frontend não compilado") {
		t.Errorf("/ = %d %s", code, body)
	}
	if code, _ := fetch(t, base+"/healthz"); code != 200 {
		t.Errorf("/healthz sem frontend = %d", code)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), `"msg":"web.not_built"`) {
		t.Errorf("sem aviso de frontend ausente: %s", logs.String())
	}
}

func TestStartGivesUpWhenContextEnds(t *testing.T) {
	// Porta fechada: o retry de 30 s termina antes, com o contexto (SIGTERM).
	cfg := testConfig(t, "DATABASE_DSN", "app:app@tcp(127.0.0.1:1)/app")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := Start(ctx, cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("Start sem banco deveria falhar")
	}
}

func TestNewLoggerIsJSONAndDefault(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelWarn)
	logger.Info("escondido")
	slog.Warn("visivel", "k", "v")
	out := buf.String()
	if strings.Contains(out, "escondido") || !strings.Contains(out, `"msg":"visivel"`) || !strings.Contains(out, `"k":"v"`) {
		t.Errorf("saída = %q", out)
	}
}

// --- integração (TEST_MYSQL_DSN) ---

func configFor(t *testing.T, mc *mysql.Config, kv ...string) *config.Config {
	t.Helper()
	return testConfig(t, append([]string{"DATABASE_DSN", dbtest.DSN(mc)}, kv...)...)
}

func TestStartMigratesBootstrapsAndServesReady(t *testing.T) {
	mc := dbtest.NewDatabase(t)
	logs := &syncBuffer{}
	a, err := Start(t.Context(), configFor(t, mc), newLogger(logs))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.OrgUnitID == 0 {
		t.Fatal("gerência padrão sem id")
	}
	var code, name string
	if err := a.DB.QueryRow("SELECT code, name FROM org_units WHERE id = ?", a.OrgUnitID).Scan(&code, &name); err != nil ||
		code != "PLATDIST" || name != "Plataformas de Distribuição" {
		t.Errorf("gerência = %q %q %v", code, name, err)
	}
	for _, msg := range []string{"db.connected", "migrate.applied", "bootstrap.org_unit_created"} {
		if !strings.Contains(logs.String(), `"msg":"`+msg+`"`) {
			t.Errorf("log sem %s", msg)
		}
	}

	base, stop := startServe(t, a, fstest.MapFS{"index.html": {Data: []byte("spa")}})
	status, body := fetch(t, base+"/readyz")
	var got map[string]any
	_ = json.Unmarshal([]byte(body), &got)
	ms, _ := migrate.Embedded()
	if status != 200 || got["status"] != "ok" || got["schemaVersion"] != float64(ms[len(ms)-1].Version) {
		t.Errorf("/readyz = %d %s", status, body)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	// Segunda subida (outra unidade, ou um restart): nada a migrar, mesma
	// gerência, mesmo com o nome da variável diferente.
	logs2 := &syncBuffer{}
	b, err := Start(t.Context(), configFor(t, mc, "DEFAULT_ORG_UNIT_NAME", "Outro nome"), newLogger(logs2))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var n int
	if err := b.DB.QueryRow("SELECT COUNT(*) FROM org_units").Scan(&n); err != nil || n != 1 || b.OrgUnitID != a.OrgUnitID {
		t.Errorf("bootstrap não idempotente: %d gerências, id %d vs %d", n, b.OrgUnitID, a.OrgUnitID)
	}
	if strings.Contains(logs2.String(), "migrate.applied") || !strings.Contains(logs2.String(), "bootstrap.org_unit_name_differs") {
		t.Errorf("segunda subida: %s", logs2.String())
	}
}

func TestStartWithoutMigrateOnStart(t *testing.T) {
	t.Run("schema atrasado: recusa", func(t *testing.T) {
		mc := dbtest.NewDatabase(t)
		_, err := Start(t.Context(), configFor(t, mc, "MIGRATE_ON_START", "false"), slog.New(slog.DiscardHandler))
		if !errors.Is(err, migrate.ErrBehind) || !strings.Contains(err.Error(), "MIGRATE_ON_START=false") {
			t.Fatalf("err = %v, want ErrBehind", err)
		}
	})
	t.Run("schema dirty: recusa", func(t *testing.T) {
		mc := dbtest.NewDatabase(t)
		if _, err := migrate.UpConfig(t.Context(), mc, slog.New(slog.DiscardHandler)); err != nil {
			t.Fatal(err)
		}
		h := dbtest.Open(t, mc)
		if _, err := h.Exec("UPDATE schema_migrations SET dirty = 1 WHERE version = 1"); err != nil {
			t.Fatal(err)
		}
		_, err := Start(t.Context(), configFor(t, mc, "MIGRATE_ON_START", "false"), slog.New(slog.DiscardHandler))
		if !errors.Is(err, migrate.ErrDirty) {
			t.Fatalf("err = %v, want ErrDirty", err)
		}
		// Com MIGRATE_ON_START=true também: dirty exige migrate force.
		if _, err := Start(t.Context(), configFor(t, mc), slog.New(slog.DiscardHandler)); !errors.Is(err, migrate.ErrDirty) {
			t.Fatalf("err = %v, want ErrDirty", err)
		}
	})
	t.Run("schema à frente: avisa e sobe", func(t *testing.T) {
		mc := dbtest.NewDatabase(t)
		if _, err := migrate.UpConfig(t.Context(), mc, slog.New(slog.DiscardHandler)); err != nil {
			t.Fatal(err)
		}
		h := dbtest.Open(t, mc)
		if _, err := h.Exec("INSERT INTO schema_migrations (version, name, checksum, dirty, applied_at) VALUES (9999, 'futura', REPEAT('x', 32), 0, NOW(3))"); err != nil {
			t.Fatal(err)
		}
		logs := &syncBuffer{}
		a, err := Start(t.Context(), configFor(t, mc, "MIGRATE_ON_START", "false"), newLogger(logs))
		if err != nil {
			t.Fatal(err)
		}
		defer a.Close()
		if !strings.Contains(logs.String(), `"msg":"migrate.schema_ahead"`) {
			t.Errorf("sem aviso: %s", logs.String())
		}
		if v, err := a.ready(t.Context()); err != nil || v != 9999 {
			t.Errorf("ready = %d %v", v, err)
		}
	})
}

func TestStartChecksMaxAllowedPacket(t *testing.T) {
	mc := dbtest.NewDatabase(t)
	// 1 GB por anexo não cabe no max_allowed_packet de 64 MB do MySQL local.
	_, err := Start(t.Context(), configFor(t, mc, "FILES_MAX_BYTES", "1073741824"), slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), "max_allowed_packet") {
		t.Fatalf("err = %v", err)
	}
	// Com S3, o limite do pacote não importa.
	a, err := Start(t.Context(), configFor(t, mc, "FILES_MAX_BYTES", "1073741824", "FILES_BACKEND", "s3",
		"S3_ENDPOINT", "s3.local:9000", "S3_BUCKET", "anexos"), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
}

func TestReadyReportsSchemaProblem(t *testing.T) {
	mc := dbtest.NewDatabase(t)
	logs := &syncBuffer{}
	a, err := Start(t.Context(), configFor(t, mc), newLogger(logs))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.DB.Exec("UPDATE schema_migrations SET dirty = 1 WHERE version = 1"); err != nil {
		t.Fatal(err)
	}
	base, stop := startServe(t, a, fstest.MapFS{"index.html": {Data: []byte("spa")}})
	defer stop()
	if code, body := fetch(t, base+"/readyz"); code != 503 || !strings.Contains(body, "schema do banco não é compatível") {
		t.Errorf("/readyz com dirty = %d %s", code, body)
	}
}
