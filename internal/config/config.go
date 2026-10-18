// Package config carrega a configuração das variáveis de ambiente (SDD §10.4)
// e aplica as travas de produção (SDD §7.9). Segredos usam o tipo Secret,
// redigido em logs e em qualquer impressão.
package config

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Env é o ambiente de execução (APP_ENV).
type Env string

const (
	EnvProduction Env = "production"
	EnvStaging    Env = "staging"
	EnvDev        Env = "dev"
	EnvTest       Env = "test"
)

// DevLike vale para dev e test, os únicos ambientes que aceitam cookies sem
// Secure e IdP local.
func (e Env) DevLike() bool { return e == EnvDev || e == EnvTest }

func (e Env) valid() bool {
	switch e {
	case EnvProduction, EnvStaging, EnvDev, EnvTest:
		return true
	}
	return false
}

// Backends de arquivo (FILES_BACKEND).
const (
	FilesMySQL = "mysql"
	FilesS3    = "s3"
)

// SessionKeySize é o tamanho exigido de SESSION_ENC_KEY depois de decodificada
// (AES-256).
const SessionKeySize = 32

// Config é a configuração completa do processo. Construída por Load/FromEnv;
// testes podem montá-la à mão e chamar Validate.
type Config struct {
	Env      Env
	Port     int
	BaseURL  string // APP_BASE_URL sem barra final, ex.: https://processos.exemplo.com
	TZName   string
	Location *time.Location
	LogLevel slog.Level

	DatabaseDSN       Secret // DATABASE_DSN (formato go-sql-driver)
	DatabaseURLEnv    string // nome da variável com a URL do DBaaS
	DatabaseURL       Secret // valor da variável DatabaseURLEnv (mysql://…)
	MigrationDSN      Secret
	MigrateOnStart    bool
	DBTLSCAPEM        string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	OIDCIssuerURL       string
	OIDCClientID        string
	OIDCClientSecret    Secret // vazio = cliente público com PKCE
	OIDCScopes          []string
	OIDCGroupsClaim     string
	OIDCEmailClaim      string
	OIDCNameClaim       string
	OIDCEmployeeIDClaim string

	RoleGroupsManager []string
	RoleGroupsHead    []string
	RoleGroupsAdmin   []string

	SessionEncKey          Secret // base64 de 32 B
	SessionEncKeyPrevious  Secret // só decifra (rotação)
	SessionIdleTimeout     time.Duration
	SessionAbsoluteTimeout time.Duration
	SessionNoRefreshMax    time.Duration
	RoleRecheckInterval    time.Duration
	RoleRecheckGrace       time.Duration
	CookieSecure           bool

	TrustedProxyCIDRs    []netip.Prefix
	RateLimitSharePerMin int
	RateLimitAuthPerMin  int

	FilesBackend      string
	FilesMaxBytes     int64
	S3Endpoint        string
	S3Bucket          string
	S3Region          string
	S3AccessKeyID     Secret
	S3SecretAccessKey Secret
	S3UseSSL          bool
	S3PathStyle       bool
	ClamdAddr         string
	UploadRequireScan bool

	JobsEnabled bool

	DefaultOrgUnitCode string
	DefaultOrgUnitName string

	GupyBaseURL string
	GupyToken   Secret

	MetricsToken Secret // vazio desliga /metrics
}

// Error lista todos os problemas encontrados, para o operador corrigir tudo
// de uma vez.
type Error struct {
	Problems []string
}

func (e *Error) Error() string {
	return "configuração inválida: " + strings.Join(e.Problems, "; ")
}

// Load lê o ambiente do processo e valida.
func Load() (*Config, error) { return FromEnv(os.Getenv) }

// FromEnv lê as variáveis por getenv, aplica os defaults e valida. Variável
// vazia equivale a ausente.
func FromEnv(getenv func(string) string) (*Config, error) {
	p := &parser{getenv: getenv}
	c := &Config{
		Env:      Env(p.str("APP_ENV", string(EnvProduction))),
		Port:     p.int("PORT", 8888),
		BaseURL:  strings.TrimRight(p.str("APP_BASE_URL", ""), "/"),
		TZName:   p.str("APP_TZ", "America/Sao_Paulo"),
		LogLevel: p.level("LOG_LEVEL", slog.LevelInfo),

		DatabaseDSN:       p.secret("DATABASE_DSN"),
		DatabaseURLEnv:    p.str("DATABASE_URL_ENV", "DATABASE_URL"),
		MigrationDSN:      p.secret("MIGRATION_DATABASE_DSN"),
		MigrateOnStart:    p.bool("MIGRATE_ON_START", true),
		DBTLSCAPEM:        p.str("DB_TLS_CA_PEM", ""),
		DBMaxOpenConns:    p.int("DB_MAX_OPEN_CONNS", 10),
		DBMaxIdleConns:    p.int("DB_MAX_IDLE_CONNS", 5),
		DBConnMaxLifetime: p.duration("DB_CONN_MAX_LIFETIME", 5*time.Minute),

		OIDCIssuerURL:       strings.TrimRight(p.str("OIDC_ISSUER_URL", ""), "/"),
		OIDCClientID:        p.str("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:    p.secret("OIDC_CLIENT_SECRET"),
		OIDCScopes:          strings.Fields(strings.ReplaceAll(p.str("OIDC_SCOPES", "openid profile email"), ",", " ")),
		OIDCGroupsClaim:     p.str("OIDC_GROUPS_CLAIM", "groups"),
		OIDCEmailClaim:      p.str("OIDC_EMAIL_CLAIM", "email"),
		OIDCNameClaim:       p.str("OIDC_NAME_CLAIM", "name"),
		OIDCEmployeeIDClaim: p.str("OIDC_EMPLOYEE_ID_CLAIM", ""),

		RoleGroupsManager: p.csv("ROLE_GROUPS_MANAGER", ""),
		RoleGroupsHead:    p.csv("ROLE_GROUPS_HEAD", ""),
		RoleGroupsAdmin:   p.csv("ROLE_GROUPS_ADMIN", ""),

		SessionEncKey:          p.secret("SESSION_ENC_KEY"),
		SessionEncKeyPrevious:  p.secret("SESSION_ENC_KEY_PREVIOUS"),
		SessionIdleTimeout:     p.duration("SESSION_IDLE_TIMEOUT", 2*time.Hour),
		SessionAbsoluteTimeout: p.duration("SESSION_ABSOLUTE_TIMEOUT", 12*time.Hour),
		SessionNoRefreshMax:    p.duration("SESSION_NO_REFRESH_MAX", 4*time.Hour),
		RoleRecheckInterval:    p.duration("ROLE_RECHECK_INTERVAL", 15*time.Minute),
		RoleRecheckGrace:       p.duration("ROLE_RECHECK_GRACE", time.Hour),
		CookieSecure:           p.bool("COOKIE_SECURE", true),

		TrustedProxyCIDRs:    p.cidrs("TRUSTED_PROXY_CIDRS", "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8"),
		RateLimitSharePerMin: p.int("RATE_LIMIT_SHARE_PER_MIN", 30),
		RateLimitAuthPerMin:  p.int("RATE_LIMIT_AUTH_PER_MIN", 30),

		FilesBackend:      p.str("FILES_BACKEND", FilesMySQL),
		FilesMaxBytes:     int64(p.int("FILES_MAX_BYTES", 10<<20)),
		S3Endpoint:        p.str("S3_ENDPOINT", ""),
		S3Bucket:          p.str("S3_BUCKET", ""),
		S3Region:          p.str("S3_REGION", ""),
		S3AccessKeyID:     p.secret("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey: p.secret("S3_SECRET_ACCESS_KEY"),
		S3UseSSL:          p.bool("S3_USE_SSL", true),
		S3PathStyle:       p.bool("S3_PATH_STYLE", true),
		ClamdAddr:         p.str("CLAMD_ADDR", ""),
		UploadRequireScan: p.bool("UPLOAD_REQUIRE_SCAN", false),

		JobsEnabled: p.bool("JOBS_ENABLED", true),

		DefaultOrgUnitCode: p.str("DEFAULT_ORG_UNIT_CODE", ""),
		DefaultOrgUnitName: p.str("DEFAULT_ORG_UNIT_NAME", ""),

		GupyBaseURL: p.str("GUPY_BASE_URL", ""),
		GupyToken:   p.secret("GUPY_TOKEN"),

		MetricsToken: p.secret("METRICS_TOKEN"),
	}
	if c.DatabaseURLEnv != "" {
		c.DatabaseURL = p.secret(c.DatabaseURLEnv)
	}
	if loc, err := time.LoadLocation(c.TZName); err != nil {
		p.add("APP_TZ", "fuso horário desconhecido %q", c.TZName)
	} else {
		c.Location = loc
	}

	// Um erro de leitura já explica a variável; não repete o da validação,
	// que veria o default no lugar do valor recusado.
	v := &parser{}
	c.validate(v)
	problems := p.problems
	for i, key := range v.keys {
		if !slices.Contains(p.keys, key) {
			problems = append(problems, v.problems[i])
		}
	}
	if len(problems) > 0 {
		return nil, &Error{Problems: problems}
	}
	return c, nil
}

// Validate confere as regras semânticas e as travas de produção. Devolve
// *Error com todos os problemas, ou nil.
func (c *Config) Validate() error {
	v := &parser{}
	c.validate(v)
	if len(v.problems) > 0 {
		return &Error{Problems: v.problems}
	}
	return nil
}

func (c *Config) validate(v *parser) {
	if !c.Env.valid() {
		v.add("APP_ENV", "valor inválido %q (use production, staging, dev ou test)", string(c.Env))
	}
	if c.Port < 1 || c.Port > 65535 {
		v.add("PORT", "deve estar entre 1 e 65535")
	}
	if c.Location == nil {
		v.add("APP_TZ", "fuso horário não carregado")
	}

	baseURL := v.httpURL("APP_BASE_URL", c.BaseURL, true)
	if baseURL != nil && (baseURL.Path != "" || baseURL.RawQuery != "" || baseURL.Fragment != "" || baseURL.User != nil) {
		v.add("APP_BASE_URL", "deve conter só esquema, host e porta (sem caminho, query ou credenciais)")
	}

	if c.DBTLSCAPEM != "" && !x509.NewCertPool().AppendCertsFromPEM([]byte(c.DBTLSCAPEM)) {
		v.add("DB_TLS_CA_PEM", "nenhum certificado PEM válido")
	}
	if c.DBMaxOpenConns < 1 {
		v.add("DB_MAX_OPEN_CONNS", "deve ser maior que zero")
	}
	if c.DBMaxIdleConns < 0 {
		v.add("DB_MAX_IDLE_CONNS", "não pode ser negativo")
	}
	v.positive("DB_CONN_MAX_LIFETIME", c.DBConnMaxLifetime)

	issuer := v.httpURL("OIDC_ISSUER_URL", c.OIDCIssuerURL, true)
	if c.OIDCClientID == "" {
		v.add("OIDC_CLIENT_ID", "obrigatória")
	}
	if !slices.Contains(c.OIDCScopes, "openid") {
		v.add("OIDC_SCOPES", "deve incluir openid")
	}
	if c.OIDCGroupsClaim == "" {
		v.add("OIDC_GROUPS_CLAIM", "obrigatória")
	}
	if c.OIDCEmailClaim == "" {
		v.add("OIDC_EMAIL_CLAIM", "obrigatória")
	}
	if c.OIDCNameClaim == "" {
		v.add("OIDC_NAME_CLAIM", "obrigatória")
	}
	if len(c.RoleGroupsManager) == 0 {
		v.add("ROLE_GROUPS_MANAGER", "obrigatória: sem grupo de gestores ninguém passa pelo portão de entrada")
	}

	if !c.SessionEncKey.IsSet() {
		v.add("SESSION_ENC_KEY", "obrigatória (base64 de %d bytes)", SessionKeySize)
	} else if _, err := DecodeKey(c.SessionEncKey); err != nil {
		v.add("SESSION_ENC_KEY", "%v", err)
	}
	if c.SessionEncKeyPrevious.IsSet() {
		if _, err := DecodeKey(c.SessionEncKeyPrevious); err != nil {
			v.add("SESSION_ENC_KEY_PREVIOUS", "%v", err)
		}
	}
	v.positive("SESSION_IDLE_TIMEOUT", c.SessionIdleTimeout)
	v.positive("SESSION_ABSOLUTE_TIMEOUT", c.SessionAbsoluteTimeout)
	v.positive("SESSION_NO_REFRESH_MAX", c.SessionNoRefreshMax)
	v.positive("ROLE_RECHECK_INTERVAL", c.RoleRecheckInterval)
	v.positive("ROLE_RECHECK_GRACE", c.RoleRecheckGrace)
	if !c.CookieSecure && !c.Env.DevLike() {
		v.add("COOKIE_SECURE", "false só é permitido com APP_ENV=dev ou test")
	}

	if c.RateLimitSharePerMin < 1 {
		v.add("RATE_LIMIT_SHARE_PER_MIN", "deve ser maior que zero")
	}
	if c.RateLimitAuthPerMin < 1 {
		v.add("RATE_LIMIT_AUTH_PER_MIN", "deve ser maior que zero")
	}

	switch c.FilesBackend {
	case FilesMySQL:
	case FilesS3:
		if c.S3Endpoint == "" {
			v.add("S3_ENDPOINT", "obrigatória com FILES_BACKEND=s3")
		}
		if c.S3Bucket == "" {
			v.add("S3_BUCKET", "obrigatória com FILES_BACKEND=s3")
		}
	default:
		v.add("FILES_BACKEND", "valor inválido %q (use mysql ou s3)", c.FilesBackend)
	}
	if c.FilesMaxBytes < 1 {
		v.add("FILES_MAX_BYTES", "deve ser maior que zero")
	}
	if c.UploadRequireScan && c.ClamdAddr == "" {
		v.add("UPLOAD_REQUIRE_SCAN", "exige CLAMD_ADDR")
	}

	switch {
	case c.DefaultOrgUnitCode == "":
		v.add("DEFAULT_ORG_UNIT_CODE", "obrigatória")
	case len([]rune(c.DefaultOrgUnitCode)) > 32:
		v.add("DEFAULT_ORG_UNIT_CODE", "deve ter no máximo 32 caracteres")
	}
	switch {
	case c.DefaultOrgUnitName == "":
		v.add("DEFAULT_ORG_UNIT_NAME", "obrigatória")
	case len([]rune(c.DefaultOrgUnitName)) > 200:
		v.add("DEFAULT_ORG_UNIT_NAME", "deve ter no máximo 200 caracteres")
	}

	if c.GupyBaseURL != "" {
		v.httpURL("GUPY_BASE_URL", c.GupyBaseURL, false)
	}

	// Travas de produção (§7.9). Chave curta, grupo de gestores vazio e
	// gerência padrão ausente já são recusados acima em qualquer ambiente.
	if c.Env == EnvProduction {
		if baseURL != nil && baseURL.Scheme != "https" {
			v.add("APP_BASE_URL", "deve usar https em produção")
		}
		if issuer != nil {
			if issuer.Scheme != "https" {
				v.add("OIDC_ISSUER_URL", "deve usar https em produção")
			}
			if isLocalhost(issuer.Hostname()) {
				v.add("OIDC_ISSUER_URL", "não pode apontar para localhost em produção")
			}
		}
	}
}

// HTTPS informa se APP_BASE_URL usa https (liga o HSTS).
func (c *Config) HTTPS() bool { return strings.HasPrefix(c.BaseURL, "https://") }

// LogAttrs resume a configuração sem segredos, para o log de partida.
func (c *Config) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("env", string(c.Env)),
		slog.Int("port", c.Port),
		slog.String("base_url", c.BaseURL),
		slog.String("tz", c.TZName),
		slog.String("oidc_issuer", c.OIDCIssuerURL),
		slog.Bool("cookie_secure", c.CookieSecure),
		slog.Bool("migrate_on_start", c.MigrateOnStart),
		slog.Bool("jobs_enabled", c.JobsEnabled),
		slog.String("files_backend", c.FilesBackend),
		slog.Bool("metrics_enabled", c.MetricsToken.IsSet()),
	}
}

// DecodeKey decodifica uma chave base64 (padrão ou URL, com ou sem padding) e
// exige SessionKeySize bytes.
func DecodeKey(s Secret) ([]byte, error) {
	raw := s.Reveal()
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		key, err := enc.DecodeString(raw)
		if err != nil {
			continue
		}
		if len(key) != SessionKeySize {
			return nil, fmt.Errorf("deve ter %d bytes depois de decodificada (tem %d)", SessionKeySize, len(key))
		}
		return key, nil
	}
	return nil, fmt.Errorf("não é base64 válido")
}

func isLocalhost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.IsLoopback() || addr.IsUnspecified()
	}
	return false
}

// parser coleta problemas de leitura e de validação.
type parser struct {
	getenv   func(string) string
	keys     []string // variável de cada problema, na mesma ordem
	problems []string
}

func (p *parser) add(key, format string, args ...any) {
	p.keys = append(p.keys, key)
	p.problems = append(p.problems, key+": "+fmt.Sprintf(format, args...))
}

func (p *parser) lookup(key string) (string, bool) {
	v := strings.TrimSpace(p.getenv(key))
	return v, v != ""
}

func (p *parser) str(key, def string) string {
	if v, ok := p.lookup(key); ok {
		return v
	}
	return def
}

func (p *parser) secret(key string) Secret {
	v, _ := p.lookup(key)
	return Secret(v)
}

func (p *parser) int(key string, def int) int {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.add(key, "deve ser um número inteiro (recebido %q)", v)
		return def
	}
	return n
}

func (p *parser) bool(key string, def bool) bool {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.add(key, "deve ser true ou false (recebido %q)", v)
		return def
	}
	return b
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		p.add(key, "duração inválida %q (exemplos: 90s, 15m, 2h)", v)
		return def
	}
	return d
}

func (p *parser) level(key string, def slog.Level) slog.Level {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		p.add(key, "nível inválido %q (use debug, info, warn ou error)", v)
		return def
	}
	return l
}

func (p *parser) csv(key, def string) []string {
	var out []string
	for _, part := range strings.Split(p.str(key, def), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (p *parser) cidrs(key, def string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range p.csv(key, def) {
		pfx, err := netip.ParsePrefix(s)
		if err != nil {
			p.add(key, "CIDR inválido %q", s)
			continue
		}
		out = append(out, pfx.Masked())
	}
	return out
}

func (p *parser) positive(key string, d time.Duration) {
	if d <= 0 {
		p.add(key, "deve ser maior que zero")
	}
}

// httpURL valida uma URL absoluta http(s) e a devolve já analisada (nil se
// ausente ou inválida).
func (p *parser) httpURL(key, raw string, required bool) *url.URL {
	if raw == "" {
		if required {
			p.add(key, "obrigatória")
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p.add(key, "deve ser uma URL absoluta http ou https (recebido %q)", raw)
		return nil
	}
	if _, port, err := net.SplitHostPort(u.Host); err == nil && port == "" {
		p.add(key, "porta vazia em %q", raw)
		return nil
	}
	return u
}
