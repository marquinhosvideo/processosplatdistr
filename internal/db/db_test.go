package db

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/marquinhosvideo/processosplatdistr/internal/config"
)

func TestNewConfigAppliesFixedParams(t *testing.T) {
	// Parâmetros do DSN que contrariam a §10.3 são sobrescritos.
	mc, err := NewConfig("app:app@tcp(db.local)/app?parseTime=false&loc=Local&multiStatements=true&collation=latin1_swedish_ci&time_zone=%27-03%3A00%27&maxAllowedPacket=1024&timeout=1s&autocommit=1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !mc.ParseTime || mc.Loc != time.UTC || mc.MultiStatements || mc.Collation != Collation || mc.MaxAllowedPacket != 0 {
		t.Errorf("parseTime=%v loc=%v multi=%v collation=%q packet=%d", mc.ParseTime, mc.Loc, mc.MultiStatements, mc.Collation, mc.MaxAllowedPacket)
	}
	if mc.Timeout != 5*time.Second || mc.ReadTimeout != 30*time.Second || mc.WriteTimeout != 30*time.Second {
		t.Errorf("timeouts = %v %v %v", mc.Timeout, mc.ReadTimeout, mc.WriteTimeout)
	}
	if mc.Params["time_zone"] != "'+00:00'" || mc.Params["sql_mode"] != "'"+SQLMode+"'" {
		t.Errorf("params = %v", mc.Params)
	}
	if mc.Params["autocommit"] != "1" {
		t.Errorf("parâmetro de sessão do DSN perdido: %v", mc.Params)
	}
	if mc.Addr != "db.local:3306" || mc.DBName != "app" || mc.User != "app" || mc.Passwd != "app" {
		t.Errorf("addr=%q db=%q user=%q", mc.Addr, mc.DBName, mc.User)
	}
	if !strings.Contains(SQLMode, "STRICT_TRANS_TABLES") || !strings.Contains(SQLMode, "ONLY_FULL_GROUP_BY") {
		t.Errorf("sql_mode = %q", SQLMode)
	}
	// O driver aceita a config resultante.
	if _, err := mysql.NewConnector(mc); err != nil {
		t.Errorf("NewConnector: %v", err)
	}
}

func TestNewConfigTLS(t *testing.T) {
	t.Run("preferred por padrão", func(t *testing.T) {
		mc, err := NewConfig("u:p@tcp(db:3306)/app", "")
		if err != nil {
			t.Fatal(err)
		}
		if mc.TLSConfig != "preferred" || mc.TLS == nil || !mc.TLS.InsecureSkipVerify || !mc.AllowFallbackToPlaintext {
			t.Errorf("tls=%q %+v fallback=%v", mc.TLSConfig, mc.TLS, mc.AllowFallbackToPlaintext)
		}
	})
	t.Run("tls explícito no DSN é respeitado", func(t *testing.T) {
		mc, err := NewConfig("u:p@tcp(db:3306)/app?tls=true", "")
		if err != nil {
			t.Fatal(err)
		}
		if mc.TLSConfig != "true" || mc.TLS == nil || mc.TLS.InsecureSkipVerify || mc.AllowFallbackToPlaintext {
			t.Errorf("tls=%q %+v", mc.TLSConfig, mc.TLS)
		}
	})
	t.Run("CA do DBaaS verifica cadeia e host", func(t *testing.T) {
		mc, err := NewConfig("u:p@tcp(mysql.dbaas.interno:3306)/app?tls=skip-verify", testCAPEM(t))
		if err != nil {
			t.Fatal(err)
		}
		if mc.TLSConfig != TLSConfigName || mc.TLS == nil || mc.TLS.InsecureSkipVerify || mc.AllowFallbackToPlaintext {
			t.Fatalf("tls=%q %+v fallback=%v", mc.TLSConfig, mc.TLS, mc.AllowFallbackToPlaintext)
		}
		if mc.TLS.ServerName != "mysql.dbaas.interno" || mc.TLS.RootCAs == nil {
			t.Errorf("serverName=%q rootCAs=%v", mc.TLS.ServerName, mc.TLS.RootCAs)
		}
		// O nome registrado no driver também funciona num DSN em texto.
		if _, err := mysql.ParseDSN("u:p@tcp(mysql.dbaas.interno:3306)/app?tls=" + TLSConfigName); err != nil {
			t.Errorf("tls=%s não registrado: %v", TLSConfigName, err)
		}
	})
	t.Run("CA inválida", func(t *testing.T) {
		if _, err := NewConfig("u:p@tcp(db:3306)/app", "não é PEM"); err == nil || !strings.Contains(err.Error(), "DB_TLS_CA_PEM") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestNewConfigRejectsBadDSN(t *testing.T) {
	for _, raw := range []string{
		"sem-barra",
		"u:p@tcp(db:3306/app",
		"u:p@udp(db:3306)/app",
		"mysql://",
		"mysql:///app",
		"postgres://u:p@db/app",
		"mysql://u:p@db/app/extra",
		"mysql://u:p@db/app#frag",
		"mysql://u:p@db:abc/app",
	} {
		if mc, err := NewConfig(raw, ""); err == nil {
			t.Errorf("%q aceito: %+v", raw, mc)
		}
	}
}

func TestURLToDSN(t *testing.T) {
	tests := []struct{ in, want string }{
		{"mysql://app:segredo@db.interno:3307/processos", "app:segredo@tcp(db.interno:3307)/processos"},
		{"mysql://app:segredo@db.interno/processos", "app:segredo@tcp(db.interno:3306)/processos"},
		{"MYSQL://app@db/processos?tls=true&charset=utf8mb4", "app@tcp(db:3306)/processos?charset=utf8mb4&tls=true"},
		{"mysql://db/processos", "tcp(db:3306)/processos"},
		{"mysql://app:s%40e%2Fn%3Aha@[::1]:3306/processos", "app:s@e/n:ha@tcp([::1]:3306)/processos"},
		{"mysql://app:p@db/", "app:p@tcp(db:3306)/"},
	}
	for _, tt := range tests {
		got, err := URLToDSN(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("URLToDSN(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestURLWithSpecialPasswordRoundTrips(t *testing.T) {
	mc, err := NewConfig("mysql://app:p%40ss%2F%3Aw%28rd%29@db.interno:3306/meu_banco?readTimeout=1s", "")
	if err != nil {
		t.Fatal(err)
	}
	if mc.User != "app" || mc.Passwd != "p@ss/:w(rd)" || mc.Addr != "db.interno:3306" || mc.DBName != "meu_banco" {
		t.Errorf("user=%q pass=%q addr=%q db=%q", mc.User, mc.Passwd, mc.Addr, mc.DBName)
	}
}

func TestURLErrorsDoNotLeakPassword(t *testing.T) {
	for _, raw := range []string{"mysql://app:segredo@db:x/app", "mysql://app:segredo@db/a/b", "mysql://app:segredo@db/app?%zz"} {
		_, err := NewConfig(raw, "")
		if err == nil {
			t.Fatalf("%q aceito", raw)
		}
		if strings.Contains(err.Error(), "segredo") {
			t.Errorf("erro vaza a senha: %v", err)
		}
	}
}

func TestFromConfig(t *testing.T) {
	base := func() *config.Config {
		return &config.Config{DatabaseURLEnv: "DATABASE_URL"}
	}

	c := base()
	c.DatabaseDSN = "app:app@tcp(db:3306)/app"
	c.DatabaseURL = "mysql://outro:x@outro/outro"
	app, mig, err := FromConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	if app.User != "app" || mig.User != "app" || app == mig {
		t.Errorf("DSN deveria ter precedência e a migração ser uma cópia: %q %q", app.User, mig.User)
	}

	c = base()
	c.DatabaseURLEnv = "DBAAS_MYSQL_ENDPOINT"
	c.DatabaseURL = "mysql://dbaas:x@dbaas-host:3306/prod"
	c.MigrationDSN = "ddl:y@tcp(dbaas-host:3306)/prod"
	app, mig, err = FromConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	if app.User != "dbaas" || mig.User != "ddl" || mig.MultiStatements {
		t.Errorf("app=%q mig=%q multi=%v", app.User, mig.User, mig.MultiStatements)
	}

	c = base()
	c.DatabaseURLEnv = "DBAAS_MYSQL_ENDPOINT"
	c.DatabaseURL = "mysql://dbaas:segredo@host:porta/prod"
	if _, _, err := FromConfig(c); err == nil || !strings.HasPrefix(err.Error(), "DBAAS_MYSQL_ENDPOINT:") || strings.Contains(err.Error(), "segredo") {
		t.Errorf("err = %v", err)
	}

	if _, _, err := FromConfig(base()); err == nil {
		t.Error("sem DSN nem URL deveria falhar")
	}
}

func TestErrorHelpers(t *testing.T) {
	dup := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'a@b.c' for key 'people.uq_people_email'"}
	wrapped := fmt.Errorf("inserir: %w", dup)
	if !IsDuplicate(wrapped) || IsFKViolation(wrapped) || IsLockNowait(wrapped) {
		t.Error("1062 classificado errado")
	}
	if key, ok := DuplicateKey(wrapped); !ok || key != "uq_people_email" {
		t.Errorf("DuplicateKey = %q %v", key, ok)
	}
	if key, ok := DuplicateKey(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry '1' for key 'PRIMARY'"}); !ok || key != "PRIMARY" {
		t.Errorf("DuplicateKey(PRIMARY) = %q %v", key, ok)
	}
	if _, ok := DuplicateKey(errors.New("x")); ok {
		t.Error("DuplicateKey de erro comum")
	}
	for _, n := range []uint16{1451, 1452, 1216, 1217} {
		if !IsFKViolation(&mysql.MySQLError{Number: n}) {
			t.Errorf("%d não é FK", n)
		}
	}
	if !IsLockNowait(fmt.Errorf("x: %w", &mysql.MySQLError{Number: 3572})) {
		t.Error("3572 não é NOWAIT")
	}
	if !IsCheckViolation(&mysql.MySQLError{Number: 3819}) {
		t.Error("3819 não é CHECK")
	}
	if Code(errors.New("x")) != 0 || IsDuplicate(nil) {
		t.Error("erro comum com código")
	}
}

func TestCheckVersion(t *testing.T) {
	for _, v := range []string{"8.0.16", "8.0.36-0ubuntu0.24.04.1", "8.4.3", "9.1.0", "8.0.46-0ubuntu0.24.04.4"} {
		if err := checkVersion(v); err != nil {
			t.Errorf("%s recusada: %v", v, err)
		}
	}
	for _, v := range []string{"8.0.15", "5.7.44-log", "10.11.6-MariaDB", "8.0", "x.y.z"} {
		if err := checkVersion(v); err == nil {
			t.Errorf("%s aceita", v)
		}
	}
}

func TestPermanentErrors(t *testing.T) {
	if !permanent(&mysql.MySQLError{Number: 1045}) || !permanent(fmt.Errorf("x: %w", &mysql.MySQLError{Number: 1049})) {
		t.Error("acesso negado deveria ser permanente")
	}
	if permanent(&mysql.MySQLError{Number: 1040}) || permanent(errors.New("connection refused")) {
		t.Error("erro transitório tratado como permanente")
	}
}

// testCAPEM gera uma CA autoassinada só para o parse do PEM.
func testCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CA de teste"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
