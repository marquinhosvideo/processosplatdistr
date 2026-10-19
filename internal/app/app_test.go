package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/config"
)

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

func testConfig(t *testing.T) *config.Config {
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
	cfg, err := config.FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func startServe(t *testing.T, dist fstest.MapFS) (base string, logs *syncBuffer, stop func() error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	logs = &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, testConfig(t), logger, ln, dist) }()
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
	return "http://" + ln.Addr().String(), logs, stop
}

func fetch(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestServeAndGracefulShutdown(t *testing.T) {
	base, logs, stop := startServe(t, fstest.MapFS{"index.html": {Data: []byte("<!doctype html>spa")}})

	if code, body := fetch(t, base+"/healthz"); code != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("/healthz = %d %s", code, body)
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
	base, logs, stop := startServe(t, fstest.MapFS{"robots.txt": {Data: []byte("User-agent: *\n")}})
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
