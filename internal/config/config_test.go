package config

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"
)

// devKey é uma chave de 32 B só para testes.
var devKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func devEnv() map[string]string {
	return map[string]string{
		"APP_ENV":               "dev",
		"APP_BASE_URL":          "http://localhost:8080",
		"COOKIE_SECURE":         "false",
		"OIDC_ISSUER_URL":       "http://localhost:9000",
		"OIDC_CLIENT_ID":        "processos-dev",
		"OIDC_CLIENT_SECRET":    "dev-secret",
		"ROLE_GROUPS_MANAGER":   "gestores",
		"ROLE_GROUPS_HEAD":      "head",
		"ROLE_GROUPS_ADMIN":     "admins",
		"SESSION_ENC_KEY":       devKey,
		"DEFAULT_ORG_UNIT_CODE": "PLATDIST",
		"DEFAULT_ORG_UNIT_NAME": "Plataformas de Distribuição",
	}
}

func prodEnv() map[string]string {
	return map[string]string{
		"APP_BASE_URL":          "https://processos.exemplo.com.br",
		"OIDC_ISSUER_URL":       "https://sso.exemplo.com.br/realms/corp",
		"OIDC_CLIENT_ID":        "processos",
		"OIDC_CLIENT_SECRET":    "s3cr3t-do-idp",
		"ROLE_GROUPS_MANAGER":   "/gestores",
		"SESSION_ENC_KEY":       devKey,
		"DEFAULT_ORG_UNIT_CODE": "PLATDIST",
		"DEFAULT_ORG_UNIT_NAME": "Plataformas de Distribuição",
		"DATABASE_DSN":          "app:senha-do-banco@tcp(db:3306)/app",
		"METRICS_TOKEN":         "token-de-metricas",
	}
}

func load(env map[string]string) (*Config, error) {
	return FromEnv(func(k string) string { return env[k] })
}

func with(env map[string]string, kv ...string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	for i := 0; i < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func problems(t *testing.T, err error) []string {
	t.Helper()
	var ce *Error
	if !errors.As(err, &ce) {
		t.Fatalf("erro = %v, want *config.Error", err)
	}
	return ce.Problems
}

func hasProblem(list []string, prefix string) bool {
	return slices.ContainsFunc(list, func(p string) bool { return strings.HasPrefix(p, prefix) })
}

func TestDefaults(t *testing.T) {
	c, err := load(devEnv())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Port != 8888 || c.TZName != "America/Sao_Paulo" || c.Location == nil || c.LogLevel != slog.LevelInfo {
		t.Errorf("defaults básicos errados: port=%d tz=%q loc=%v level=%v", c.Port, c.TZName, c.Location, c.LogLevel)
	}
	if c.SessionIdleTimeout != 2*time.Hour || c.SessionAbsoluteTimeout != 12*time.Hour || c.SessionNoRefreshMax != 4*time.Hour {
		t.Errorf("timeouts de sessão errados: %v %v %v", c.SessionIdleTimeout, c.SessionAbsoluteTimeout, c.SessionNoRefreshMax)
	}
	if c.RoleRecheckInterval != 15*time.Minute || c.RoleRecheckGrace != time.Hour {
		t.Errorf("recheck errado: %v %v", c.RoleRecheckInterval, c.RoleRecheckGrace)
	}
	if !c.MigrateOnStart || !c.JobsEnabled || c.UploadRequireScan {
		t.Errorf("flags erradas: migrate=%v jobs=%v scan=%v", c.MigrateOnStart, c.JobsEnabled, c.UploadRequireScan)
	}
	if c.DBMaxOpenConns != 10 || c.DBMaxIdleConns != 5 || c.DBConnMaxLifetime != 5*time.Minute {
		t.Errorf("pool errado: %d %d %v", c.DBMaxOpenConns, c.DBMaxIdleConns, c.DBConnMaxLifetime)
	}
	if !slices.Equal(c.OIDCScopes, []string{"openid", "profile", "email"}) {
		t.Errorf("scopes = %v", c.OIDCScopes)
	}
	if c.OIDCGroupsClaim != "groups" || c.OIDCEmailClaim != "email" || c.OIDCNameClaim != "name" || c.OIDCEmployeeIDClaim != "" {
		t.Errorf("claims erradas: %q %q %q %q", c.OIDCGroupsClaim, c.OIDCEmailClaim, c.OIDCNameClaim, c.OIDCEmployeeIDClaim)
	}
	wantCIDRs := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("127.0.0.0/8"),
	}
	if !slices.Equal(c.TrustedProxyCIDRs, wantCIDRs) {
		t.Errorf("cidrs = %v", c.TrustedProxyCIDRs)
	}
	if c.RateLimitAuthPerMin != 30 || c.RateLimitSharePerMin != 30 {
		t.Errorf("rate limits = %d %d", c.RateLimitAuthPerMin, c.RateLimitSharePerMin)
	}
	if c.FilesBackend != FilesMySQL || c.FilesMaxBytes != 10485760 || !c.S3UseSSL || !c.S3PathStyle {
		t.Errorf("files = %q %d ssl=%v path=%v", c.FilesBackend, c.FilesMaxBytes, c.S3UseSSL, c.S3PathStyle)
	}
	if c.DatabaseURLEnv != "DATABASE_URL" {
		t.Errorf("DatabaseURLEnv = %q", c.DatabaseURLEnv)
	}
	if c.HTTPS() {
		t.Error("HTTPS() = true para http://localhost")
	}
}

func TestAppEnvDefaultsToProduction(t *testing.T) {
	c, err := load(prodEnv())
	if err != nil {
		t.Fatalf("produção válida recusada: %v", err)
	}
	if c.Env != EnvProduction || !c.CookieSecure || !c.HTTPS() {
		t.Errorf("env=%q cookieSecure=%v https=%v", c.Env, c.CookieSecure, c.HTTPS())
	}
}

func TestParsing(t *testing.T) {
	c, err := load(with(devEnv(),
		"PORT", " 8080 ",
		"APP_BASE_URL", "http://localhost:5173/",
		"LOG_LEVEL", "DEBUG",
		"OIDC_SCOPES", "openid, profile email offline_access",
		"ROLE_GROUPS_MANAGER", " gestores , /time-lideres ,,",
		"TRUSTED_PROXY_CIDRS", "10.1.2.3/8",
		"ROLE_RECHECK_INTERVAL", "5s",
		"MIGRATE_ON_START", "0",
		"FILES_MAX_BYTES", "1048576",
	))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Port != 8080 || c.BaseURL != "http://localhost:5173" || c.LogLevel != slog.LevelDebug {
		t.Errorf("port=%d base=%q level=%v", c.Port, c.BaseURL, c.LogLevel)
	}
	if !slices.Equal(c.OIDCScopes, []string{"openid", "profile", "email", "offline_access"}) {
		t.Errorf("scopes = %v", c.OIDCScopes)
	}
	if !slices.Equal(c.RoleGroupsManager, []string{"gestores", "/time-lideres"}) {
		t.Errorf("manager groups = %q", c.RoleGroupsManager)
	}
	if !slices.Equal(c.TrustedProxyCIDRs, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}) {
		t.Errorf("cidrs = %v (devia mascarar)", c.TrustedProxyCIDRs)
	}
	if c.RoleRecheckInterval != 5*time.Second || c.MigrateOnStart || c.FilesMaxBytes != 1<<20 {
		t.Errorf("recheck=%v migrate=%v max=%d", c.RoleRecheckInterval, c.MigrateOnStart, c.FilesMaxBytes)
	}
}

func TestDatabaseURLEnvIndirection(t *testing.T) {
	c, err := load(with(devEnv(), "DATABASE_URL_ENV", "DBAAS_MYSQL_ENDPOINT", "DBAAS_MYSQL_ENDPOINT", "mysql://u:p@db:3306/app"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.DatabaseURL.Reveal() != "mysql://u:p@db:3306/app" {
		t.Errorf("DatabaseURL = %q", c.DatabaseURL.Reveal())
	}
}

// As travas de produção da §7.9, uma a uma, partindo de uma config válida.
func TestProductionLocks(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("curta"))
	tests := []struct {
		name string
		kv   []string
		want string
	}{
		{"base URL http", []string{"APP_BASE_URL", "http://processos.exemplo.com.br"}, "APP_BASE_URL: deve usar https em produção"},
		{"issuer http", []string{"OIDC_ISSUER_URL", "http://sso.exemplo.com.br"}, "OIDC_ISSUER_URL: deve usar https em produção"},
		{"issuer localhost", []string{"OIDC_ISSUER_URL", "https://localhost:9000"}, "OIDC_ISSUER_URL: não pode apontar para localhost"},
		{"issuer subdomínio de localhost", []string{"OIDC_ISSUER_URL", "https://idp.localhost"}, "OIDC_ISSUER_URL: não pode apontar para localhost"},
		{"issuer loopback v4", []string{"OIDC_ISSUER_URL", "https://127.0.0.1:9000"}, "OIDC_ISSUER_URL: não pode apontar para localhost"},
		{"issuer loopback v6", []string{"OIDC_ISSUER_URL", "https://[::1]:9000"}, "OIDC_ISSUER_URL: não pode apontar para localhost"},
		{"cookie inseguro", []string{"COOKIE_SECURE", "false"}, "COOKIE_SECURE: false só é permitido"},
		{"chave curta", []string{"SESSION_ENC_KEY", shortKey}, "SESSION_ENC_KEY: deve ter 32 bytes"},
		{"chave ausente", []string{"SESSION_ENC_KEY", ""}, "SESSION_ENC_KEY: obrigatória"},
		{"grupos de gestor vazios", []string{"ROLE_GROUPS_MANAGER", " , "}, "ROLE_GROUPS_MANAGER: obrigatória"},
		{"gerência sem código", []string{"DEFAULT_ORG_UNIT_CODE", ""}, "DEFAULT_ORG_UNIT_CODE: obrigatória"},
		{"gerência sem nome", []string{"DEFAULT_ORG_UNIT_NAME", ""}, "DEFAULT_ORG_UNIT_NAME: obrigatória"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(with(prodEnv(), tt.kv...))
			got := problems(t, err)
			if !hasProblem(got, tt.want) {
				t.Errorf("problemas = %q, want um começando com %q", got, tt.want)
			}
			if len(got) != 1 {
				t.Errorf("esperava exatamente 1 problema, veio %q", got)
			}
		})
	}
}

func TestDevAndTestRelaxLocks(t *testing.T) {
	for _, env := range []string{"dev", "test"} {
		t.Run(env, func(t *testing.T) {
			if _, err := load(with(devEnv(), "APP_ENV", env)); err != nil {
				t.Errorf("%s com http, localhost e COOKIE_SECURE=false recusado: %v", env, err)
			}
		})
	}
}

func TestStagingRequiresSecureCookie(t *testing.T) {
	_, err := load(with(devEnv(), "APP_ENV", "staging"))
	got := problems(t, err)
	if !hasProblem(got, "COOKIE_SECURE:") || len(got) != 1 {
		t.Errorf("problemas = %q", got)
	}
	// staging aceita http e issuer local (as travas de URL são só de produção).
	if _, err := load(with(devEnv(), "APP_ENV", "staging", "COOKIE_SECURE", "true")); err != nil {
		t.Errorf("staging com cookie seguro recusado: %v", err)
	}
}

func TestParseErrorsReportedOnce(t *testing.T) {
	_, err := load(with(devEnv(),
		"APP_ENV", "prod",
		"PORT", "oitenta",
		"COOKIE_SECURE", "talvez",
		"SESSION_IDLE_TIMEOUT", "2 horas",
		"LOG_LEVEL", "verboso",
		"TRUSTED_PROXY_CIDRS", "10.0.0.0/8,rede-interna",
		"APP_TZ", "Marte/Base",
		"FILES_BACKEND", "ftp",
	))
	got := problems(t, err)
	for _, key := range []string{"APP_ENV", "PORT", "COOKIE_SECURE", "SESSION_IDLE_TIMEOUT", "LOG_LEVEL", "TRUSTED_PROXY_CIDRS", "APP_TZ", "FILES_BACKEND"} {
		n := 0
		for _, p := range got {
			if strings.HasPrefix(p, key+":") {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s aparece %d vezes em %q", key, n, got)
		}
	}
}

func TestEmptyEnvListsEverythingMissing(t *testing.T) {
	_, err := load(map[string]string{})
	got := problems(t, err)
	for _, key := range []string{"APP_BASE_URL:", "OIDC_ISSUER_URL:", "OIDC_CLIENT_ID:", "ROLE_GROUPS_MANAGER:", "SESSION_ENC_KEY:", "DEFAULT_ORG_UNIT_CODE:", "DEFAULT_ORG_UNIT_NAME:"} {
		if !hasProblem(got, key) {
			t.Errorf("faltou %s em %q", key, got)
		}
	}
	if !strings.HasPrefix(err.Error(), "configuração inválida: ") {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestOtherRules(t *testing.T) {
	tests := []struct {
		name string
		kv   []string
		want string
	}{
		{"base URL com caminho", []string{"APP_BASE_URL", "https://x.exemplo.com/app"}, "APP_BASE_URL: deve conter só"},
		{"base URL relativa", []string{"APP_BASE_URL", "processos.exemplo.com"}, "APP_BASE_URL: deve ser uma URL absoluta"},
		{"base URL ftp", []string{"APP_BASE_URL", "ftp://x.exemplo.com"}, "APP_BASE_URL: deve ser uma URL absoluta"},
		{"porta fora da faixa", []string{"PORT", "70000"}, "PORT: deve estar entre"},
		{"sem openid", []string{"OIDC_SCOPES", "profile email"}, "OIDC_SCOPES: deve incluir openid"},
		{"sem client id", []string{"OIDC_CLIENT_ID", ""}, "OIDC_CLIENT_ID: obrigatória"},
		{"chave anterior inválida", []string{"SESSION_ENC_KEY_PREVIOUS", "%%%"}, "SESSION_ENC_KEY_PREVIOUS: não é base64"},
		{"duração zero", []string{"SESSION_ABSOLUTE_TIMEOUT", "0s"}, "SESSION_ABSOLUTE_TIMEOUT: deve ser maior que zero"},
		{"s3 sem bucket", []string{"FILES_BACKEND", "s3", "S3_ENDPOINT", "minio:9000"}, "S3_BUCKET: obrigatória"},
		{"scan sem clamd", []string{"UPLOAD_REQUIRE_SCAN", "true"}, "UPLOAD_REQUIRE_SCAN: exige CLAMD_ADDR"},
		{"código de gerência longo", []string{"DEFAULT_ORG_UNIT_CODE", strings.Repeat("X", 33)}, "DEFAULT_ORG_UNIT_CODE: deve ter no máximo 32"},
		{"CA inválida", []string{"DB_TLS_CA_PEM", "não é PEM"}, "DB_TLS_CA_PEM:"},
		{"gupy sem esquema", []string{"GUPY_BASE_URL", "api.gupy.io"}, "GUPY_BASE_URL:"},
		{"pool zerado", []string{"DB_MAX_OPEN_CONNS", "0"}, "DB_MAX_OPEN_CONNS:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(with(devEnv(), tt.kv...))
			if got := problems(t, err); !hasProblem(got, tt.want) {
				t.Errorf("problemas = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateOnHandBuiltConfig(t *testing.T) {
	c, err := load(devEnv())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate de config carregada: %v", err)
	}
	c.Location = nil
	c.Env = EnvProduction
	got := problems(t, c.Validate())
	for _, want := range []string{"APP_TZ:", "APP_BASE_URL: deve usar https", "COOKIE_SECURE:", "OIDC_ISSUER_URL: deve usar https"} {
		if !hasProblem(got, want) {
			t.Errorf("faltou %q em %q", want, got)
		}
	}
}

func TestDecodeKey(t *testing.T) {
	raw := []byte("0123456789abcdef0123456789abcdef")
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		key, err := DecodeKey(Secret(enc.EncodeToString(raw)))
		if err != nil || !bytes.Equal(key, raw) {
			t.Errorf("DecodeKey(%v) = %x, %v", enc, key, err)
		}
	}
	if _, err := DecodeKey(Secret(base64.StdEncoding.EncodeToString(raw[:31]))); err == nil {
		t.Error("chave de 31 B aceita")
	}
	if _, err := DecodeKey(Secret(base64.StdEncoding.EncodeToString(append(raw, 'x')))); err == nil {
		t.Error("chave de 33 B aceita")
	}
	if _, err := DecodeKey("@@@"); err == nil {
		t.Error("base64 inválido aceito")
	}
}

func TestSecretRedaction(t *testing.T) {
	const value = "super-secreto-123"
	s := Secret(value)
	type holder struct {
		Token Secret `json:"token"`
	}

	outputs := map[string]string{
		"%s":     fmt.Sprintf("%s", s),
		"%v":     fmt.Sprintf("%v", s),
		"%+v":    fmt.Sprintf("%+v", holder{s}),
		"%#v":    fmt.Sprintf("%#v", holder{s}),
		"%q":     fmt.Sprintf("%q", s),
		"%x":     fmt.Sprintf("%x", s),
		"%d":     fmt.Sprintf("%d", s),
		"String": s.String(),
	}
	b, _ := json.Marshal(holder{s})
	outputs["json"] = string(b)
	txt, _ := s.MarshalText()
	outputs["text"] = string(txt)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("teste", "token", s, "holder", holder{s})
	slog.New(slog.NewTextHandler(&buf, nil)).Info("teste", "token", s)
	outputs["slog"] = buf.String()

	for name, out := range outputs {
		if strings.Contains(out, value) {
			t.Errorf("%s vazou o segredo: %s", name, out)
		}
		if !strings.Contains(out, redacted) {
			t.Errorf("%s sem marcador de redação: %s", name, out)
		}
	}
	if s.Reveal() != value {
		t.Error("Reveal não devolve o valor")
	}
	if Secret("").String() != "" || Secret("").IsSet() {
		t.Error("segredo vazio deveria imprimir vazio")
	}
}

// Nenhum segredo da config aparece ao imprimir a struct inteira.
func TestConfigPrintDoesNotLeakSecrets(t *testing.T) {
	c, err := load(prodEnv())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("config", "cfg", c)
	slog.New(slog.NewJSONHandler(&buf, nil)).LogAttrs(t.Context(), slog.LevelInfo, "config", c.LogAttrs()...)
	out := fmt.Sprintf("%v %+v %#v", c, *c, *c) + buf.String()
	for _, secret := range []string{"s3cr3t-do-idp", devKey, "senha-do-banco", "token-de-metricas"} {
		if strings.Contains(out, secret) {
			t.Errorf("segredo %q vazou", secret)
		}
	}
}
