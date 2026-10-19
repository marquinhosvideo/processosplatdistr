package db

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/marquinhosvideo/processosplatdistr/internal/config"
)

// Parâmetros que NewConfig aplica sempre, por cima do que vier no DSN (§10.3).
const (
	Collation    = "utf8mb4_0900_ai_ci"
	TimeZone     = "+00:00"
	SQLMode      = "STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,ONLY_FULL_GROUP_BY"
	DialTimeout  = 5 * time.Second
	ReadTimeout  = 30 * time.Second
	WriteTimeout = 30 * time.Second

	// TLSConfigName é o nome registrado no driver para a CA do DBaaS.
	TLSConfigName = "dbaas"
)

// FromConfig resolve os DSNs da aplicação e da migração a partir da config:
// DATABASE_DSN ou, se vazia, a URL da variável DATABASE_URL_ENV; e
// MIGRATION_DATABASE_DSN, que por padrão é o DSN da aplicação. Os erros não
// repetem o valor, que traz a senha.
func FromConfig(c *config.Config) (app, migration *mysql.Config, err error) {
	raw, source := c.DatabaseDSN, "DATABASE_DSN"
	if !raw.IsSet() {
		raw, source = c.DatabaseURL, c.DatabaseURLEnv
	}
	if !raw.IsSet() {
		return nil, nil, fmt.Errorf("DATABASE_DSN vazia e sem URL do banco em %s", c.DatabaseURLEnv)
	}
	if app, err = NewConfig(raw.Reveal(), c.DBTLSCAPEM); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", source, err)
	}
	if !c.MigrationDSN.IsSet() {
		return app, app.Clone(), nil
	}
	if migration, err = NewConfig(c.MigrationDSN.Reveal(), c.DBTLSCAPEM); err != nil {
		return nil, nil, fmt.Errorf("MIGRATION_DATABASE_DSN: %w", err)
	}
	return app, migration, nil
}

// PoolFromConfig lê DB_MAX_OPEN_CONNS, DB_MAX_IDLE_CONNS e DB_CONN_MAX_LIFETIME.
func PoolFromConfig(c *config.Config) Pool {
	return Pool{MaxOpen: c.DBMaxOpenConns, MaxIdle: c.DBMaxIdleConns, MaxLifetime: c.DBConnMaxLifetime}
}

// NewConfig interpreta raw — DSN do go-sql-driver (user:pass@tcp(host:3306)/db)
// ou URL mysql://user:pass@host:3306/db — e aplica os parâmetros fixos:
// parseTime, loc=UTC, time_zone, collation, sql_mode, timeouts e
// maxAllowedPacket=0 (lido do servidor). O pool da aplicação nunca usa
// multiStatements; só o runner de migrations liga a opção na própria conexão.
//
// TLS: com caPEM, tls=dbaas com verificação da cadeia e do host; sem caPEM,
// vale o tls do DSN, se houver, e senão tls=preferred.
func NewConfig(raw, caPEM string) (*mysql.Config, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "mysql://") {
		dsn, err := URLToDSN(raw)
		if err != nil {
			return nil, err
		}
		raw = dsn
	}
	mc, err := mysql.ParseDSN(raw)
	if err != nil {
		return nil, fmt.Errorf("DSN inválido: %w", err)
	}
	if mc.Net != "tcp" && mc.Net != "unix" {
		return nil, fmt.Errorf("DSN inválido: protocolo %q não suportado (use tcp ou unix)", mc.Net)
	}

	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Collation = Collation
	if mc.Params == nil {
		mc.Params = map[string]string{}
	}
	// Valores de SET: as aspas fazem parte do valor.
	mc.Params["time_zone"] = "'" + TimeZone + "'"
	mc.Params["sql_mode"] = "'" + SQLMode + "'"
	mc.Timeout = DialTimeout
	mc.ReadTimeout = ReadTimeout
	mc.WriteTimeout = WriteTimeout
	mc.MaxAllowedPacket = 0
	mc.MultiStatements = false
	mc.AllowAllFiles = false

	switch {
	case caPEM != "":
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, errors.New("DB_TLS_CA_PEM: nenhum certificado PEM válido")
		}
		host := mc.Addr
		if h, _, err := net.SplitHostPort(mc.Addr); err == nil {
			host = h
		}
		tc := &tls.Config{RootCAs: pool, ServerName: host, MinVersion: tls.VersionTLS12}
		if err := mysql.RegisterTLSConfig(TLSConfigName, tc.Clone()); err != nil {
			return nil, fmt.Errorf("registrar TLS do banco: %w", err)
		}
		mc.TLSConfig, mc.TLS, mc.AllowFallbackToPlaintext = TLSConfigName, tc, false
	case mc.TLSConfig == "":
		// Como o tls=preferred do driver: cifra se o servidor oferecer, sem
		// verificar a cadeia, e aceita texto claro se não oferecer.
		mc.TLSConfig, mc.TLS, mc.AllowFallbackToPlaintext = "preferred", &tls.Config{InsecureSkipVerify: true}, true
	}
	return mc, nil
}

// URLToDSN converte mysql://user:pass@host:port/db?params (o formato que o
// bind do DBaaS injeta) para o DSN do go-sql-driver. A porta padrão é 3306.
func URLToDSN(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// A mensagem de url.Parse repete a URL, que traz a senha.
		return "", errors.New("URL do banco inválida")
	}
	if !strings.EqualFold(u.Scheme, "mysql") || u.Host == "" || u.Opaque != "" {
		return "", errors.New("URL do banco inválida: use mysql://usuario:senha@host:porta/banco")
	}
	if u.Fragment != "" {
		return "", errors.New("URL do banco inválida: fragmento (#) não é permitido")
	}
	host, port := u.Hostname(), u.Port()
	if host == "" {
		return "", errors.New("URL do banco inválida: host vazio")
	}
	if port == "" {
		port = "3306"
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if strings.Contains(dbName, "/") {
		return "", errors.New("URL do banco inválida: o caminho deve ter só o nome do banco")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("URL do banco inválida: parâmetros malformados")
	}

	var b strings.Builder
	if u.User != nil {
		b.WriteString(u.User.Username())
		if pass, ok := u.User.Password(); ok {
			b.WriteString(":" + pass)
		}
		b.WriteString("@")
	}
	b.WriteString("tcp(" + net.JoinHostPort(host, port) + ")/" + url.PathEscape(dbName))
	if len(query) > 0 {
		// Encode escapa "/" e "&" dos valores; o driver desfaz com QueryUnescape.
		b.WriteString("?" + query.Encode())
	}
	return b.String(), nil
}
