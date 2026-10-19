package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/buildinfo"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/dbtest"
)

func TestMain(m *testing.M) { dbtest.Main(m) }

// key32 veio de /dev/urandom: produção recusa chaves fracas ou públicas.
const key32 = "3rHfT+ZqQuL8jt++3hq0AHro93k8LoMpV3oz7kbWbe8="

func prodEnv() map[string]string {
	return map[string]string{
		"APP_BASE_URL":          "https://processos.exemplo.com.br",
		"OIDC_ISSUER_URL":       "https://sso.exemplo.com.br/realms/corp",
		"OIDC_CLIENT_ID":        "processos",
		"OIDC_CLIENT_SECRET":    "segredo-do-idp",
		"ROLE_GROUPS_MANAGER":   "gestores",
		"SESSION_ENC_KEY":       key32,
		"DEFAULT_ORG_UNIT_CODE": "PLATDIST",
		"DEFAULT_ORG_UNIT_NAME": "Plataformas de Distribuição",
		"DATABASE_DSN":          "app:app@tcp(127.0.0.1:3306)/app",
	}
}

func with(env map[string]string, kv ...string) map[string]string {
	out := map[string]string{}
	for k, v := range env {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

// lockedBuffer é seguro para o servidor escrever enquanto o teste lê.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func runWith(ctx context.Context, envMap map[string]string, args ...string) (int, *lockedBuffer, *lockedBuffer) {
	stdout, stderr := &lockedBuffer{}, &lockedBuffer{}
	code := run(ctx, args, env{getenv: func(k string) string { return envMap[k] }, stdout: stdout, stderr: stderr})
	return code, stdout, stderr
}

func TestUsage(t *testing.T) {
	tests := []struct {
		args       []string
		code       int
		wantStdout string
		wantStderr string
	}{
		{nil, exitUsage, "", "Uso: server <comando>"},
		{[]string{"help"}, exitOK, "Uso: server <comando>", ""},
		{[]string{"--help"}, exitOK, "config check", ""},
		{[]string{"desconhecido"}, exitUsage, "", `comando desconhecido "desconhecido"`},
		{[]string{"config"}, exitUsage, "", "use: server config check"},
		{[]string{"config", "show"}, exitUsage, "", "use: server config check"},
		{[]string{"serve", "extra"}, exitUsage, "", "serve não recebe argumentos"},
		{[]string{"migrate", "down"}, exitUsage, "", "use: server migrate [up|status|force N]"},
		{[]string{"migrate", "force"}, exitUsage, "", "use: server migrate [up|status|force N]"},
		{[]string{"migrate", "force", "x"}, exitUsage, "", `versão inválida "x"`},
		{[]string{"migrate", "force", "0"}, exitUsage, "", `versão inválida "0"`},
		{[]string{"migrate", "up", "extra"}, exitUsage, "", "use: server migrate"},
		{[]string{"jobs", "run", "session_cleanup"}, exitError, "", "jobs ainda não está disponível"},
	}
	for _, tt := range tests {
		code, stdout, stderr := runWith(t.Context(), nil, tt.args...)
		if code != tt.code {
			t.Errorf("%v: código %d, want %d", tt.args, code, tt.code)
		}
		if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
			t.Errorf("%v: stdout = %q", tt.args, stdout)
		}
		if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
			t.Errorf("%v: stderr = %q", tt.args, stderr)
		}
	}
}

func TestVersion(t *testing.T) {
	code, stdout, _ := runWith(t.Context(), nil, "version")
	if code != exitOK || stdout.String() != buildinfo.Version+"\n" {
		t.Errorf("código %d, stdout %q", code, stdout)
	}
}

func TestConfigCheckAcceptsSecureProduction(t *testing.T) {
	code, stdout, stderr := runWith(t.Context(), prodEnv(), "config", "check")
	if code != exitOK || !strings.Contains(stdout.String(), "configuração válida (APP_ENV=production)") {
		t.Errorf("código %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestConfigCheckRejectsInsecureProduction(t *testing.T) {
	insecure := with(prodEnv(),
		"APP_BASE_URL", "http://processos.exemplo.com.br",
		"OIDC_ISSUER_URL", "https://localhost:9000",
		"COOKIE_SECURE", "false",
		"SESSION_ENC_KEY", base64.StdEncoding.EncodeToString([]byte("curta")),
		"ROLE_GROUPS_MANAGER", "",
		"DEFAULT_ORG_UNIT_CODE", "",
	)
	code, stdout, stderr := runWith(t.Context(), insecure, "config", "check")
	if code != exitError || stdout.String() != "" {
		t.Fatalf("código %d, stdout %q", code, stdout)
	}
	for _, want := range []string{
		"configuração inválida:",
		"  - APP_BASE_URL: deve usar https em produção",
		"  - OIDC_ISSUER_URL: não pode apontar para localhost em produção",
		"  - COOKIE_SECURE: false só é permitido com APP_ENV=dev ou test",
		"  - SESSION_ENC_KEY: deve ter 32 bytes",
		"  - ROLE_GROUPS_MANAGER: obrigatória",
		"  - DEFAULT_ORG_UNIT_CODE: obrigatória",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr sem %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr.String(), "segredo-do-idp") || strings.Contains(stderr.String(), key32) {
		t.Error("segredo impresso pelo config check")
	}
}

func TestConfigCheckRejectsMalformedDatabaseURL(t *testing.T) {
	env := with(prodEnv(), "DATABASE_DSN", "", "DATABASE_URL_ENV", "DBAAS_MYSQL_ENDPOINT",
		"DBAAS_MYSQL_ENDPOINT", "mysql://app:senha-do-banco@db:porta/app")
	code, _, stderr := runWith(t.Context(), env, "config", "check")
	if code != exitError || !strings.Contains(stderr.String(), "  - DBAAS_MYSQL_ENDPOINT: URL do banco inválida") {
		t.Errorf("código %d, stderr %q", code, stderr)
	}
	if strings.Contains(stderr.String(), "senha-do-banco") {
		t.Error("senha do banco impressa pelo config check")
	}
}

func TestServeRefusesInvalidConfig(t *testing.T) {
	code, stdout, stderr := runWith(t.Context(), with(prodEnv(), "COOKIE_SECURE", "false"), "serve")
	if code != exitError || !strings.Contains(stderr.String(), "COOKIE_SECURE") || stdout.String() != "" {
		t.Errorf("código %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestServeUntilCanceled sobe o serve de verdade contra um banco vazio: as
// migrations rodam na subida (MIGRATE_ON_START=true é o padrão).
func TestServeUntilCanceled(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	port := freePort(t)
	envMap := with(prodEnv(), "PORT", fmt.Sprint(port), "DATABASE_DSN", dbtest.DSN(dbtest.NewDatabase(t)))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stdout, stderr := &lockedBuffer{}, &lockedBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"serve"}, env{getenv: func(k string) string { return envMap[k] }, stdout: stdout, stderr: stderr})
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/readyz", port)
	deadline := time.Now().Add(20 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("/readyz = %d", resp.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("servidor não subiu: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("código %d, stderr %q", code, stderr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve não encerrou")
	}
	logs := stdout.String()
	if !strings.Contains(logs, `"msg":"server.start"`) || !strings.Contains(logs, `"env":"production"`) {
		t.Errorf("log de partida ausente: %s", logs)
	}
	if strings.Contains(logs, "segredo-do-idp") || strings.Contains(logs, key32) {
		t.Errorf("segredo no log: %s", logs)
	}
}

func TestServeRefusesSchemaBehindWithoutMigrateOnStart(t *testing.T) {
	envMap := with(prodEnv(), "PORT", fmt.Sprint(freePort(t)), "MIGRATE_ON_START", "false",
		"DATABASE_DSN", dbtest.DSN(dbtest.NewDatabase(t)))
	code, stdout, _ := runWith(t.Context(), envMap, "serve")
	if code != exitError {
		t.Fatalf("código %d, want %d", code, exitError)
	}
	logs := stdout.String()
	if !strings.Contains(logs, `"msg":"server.start_failed"`) || !strings.Contains(logs, "schema atrás do binário") ||
		strings.Contains(logs, "server.listening") {
		t.Errorf("logs = %s", logs)
	}
	if strings.Contains(logs, "senha") || strings.Contains(logs, "app:app@") {
		t.Errorf("DSN no log: %s", logs)
	}
}

func TestMigrateCommands(t *testing.T) {
	// Pela URL mysql://, o formato que o bind do DBaaS injeta.
	mc := dbtest.NewDatabase(t)
	envMap := with(prodEnv(), "DATABASE_DSN", "", "DATABASE_URL",
		fmt.Sprintf("mysql://%s:%s@%s/%s", mc.User, mc.Passwd, mc.Addr, mc.DBName))
	row := regexp.MustCompile

	code, stdout, stderr := runWith(t.Context(), envMap, "migrate", "status")
	if code != exitOK || !row(`0001 +core +pendente`).MatchString(stdout.String()) || !strings.Contains(stdout.String(), "schema atrás do binário") {
		t.Fatalf("status antes: %d\n%s\n%s", code, stdout, stderr)
	}
	code, stdout, stderr = runWith(t.Context(), envMap, "migrate")
	if code != exitOK || !strings.Contains(stdout.String(), "aplicada 0001_core.sql") {
		t.Fatalf("up: %d\n%s\n%s", code, stdout, stderr)
	}
	code, stdout, _ = runWith(t.Context(), envMap, "migrate", "up")
	if code != exitOK || !strings.Contains(stdout.String(), "nada a aplicar") {
		t.Errorf("up de novo: %d %s", code, stdout)
	}
	code, stdout, _ = runWith(t.Context(), envMap, "migrate", "status")
	if code != exitOK || !row(`0001 +core +aplicada +\d{4}-`).MatchString(stdout.String()) || !strings.Contains(stdout.String(), "situação: schema na versão") {
		t.Errorf("status depois: %d\n%s", code, stdout)
	}
	code, stdout, _ = runWith(t.Context(), envMap, "migrate", "force", "1")
	if code != exitOK || !strings.Contains(stdout.String(), "versão 0001 marcada como aplicada e limpa") {
		t.Errorf("force: %d %s", code, stdout)
	}
	code, _, stderr = runWith(t.Context(), envMap, "migrate", "force", "999")
	if code != exitError || !strings.Contains(stderr.String(), "não existe neste binário") {
		t.Errorf("force inexistente: %d %s", code, stderr)
	}
}
