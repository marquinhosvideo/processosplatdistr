// Package db abre o pool MySQL da aplicação (SDD §10.3) e reúne o que todos
// os módulos usam para falar com o banco: a interface Q, InTx e os helpers de
// erro do MySQL. O SQL é escrito à mão em cada módulo.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Q é o que *sql.DB, *sql.Tx e *sql.Conn têm em comum. Funções que só
// executam SQL recebem Q, e quem chama decide se é dentro de uma transação.
type Q interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var (
	_ Q = (*sql.DB)(nil)
	_ Q = (*sql.Tx)(nil)
	_ Q = (*sql.Conn)(nil)
)

// Pool são os limites do pool de conexões (DB_MAX_OPEN_CONNS etc.).
type Pool struct {
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
}

// DefaultPool é o pool da §10.3: MaxOpen 10, MaxIdle 5, ConnMaxLifetime 5 min.
var DefaultPool = Pool{MaxOpen: 10, MaxIdle: 5, MaxLifetime: 5 * time.Minute}

// Open cria o *sql.DB para mc, já com os parâmetros de NewConfig aplicados,
// sem abrir conexão.
func Open(mc *mysql.Config, pool Pool) (*sql.DB, error) {
	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, fmt.Errorf("configuração do banco: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(pool.MaxOpen)
	db.SetMaxIdleConns(pool.MaxIdle)
	db.SetConnMaxLifetime(pool.MaxLifetime)
	return db, nil
}

// Connect abre o pool e espera o banco responder ao ping por até retryFor
// (30 s na subida do serve). Credencial recusada ou banco inexistente não
// melhoram com espera e falham na hora.
func Connect(ctx context.Context, mc *mysql.Config, pool Pool, retryFor time.Duration, logger *slog.Logger) (*sql.DB, error) {
	db, err := Open(mc, pool)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(retryFor)
	for attempt := 1; ; attempt++ {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pctx)
		cancel()
		if err == nil {
			return db, nil
		}
		if permanent(err) || ctx.Err() != nil || time.Now().After(deadline) {
			db.Close()
			return nil, fmt.Errorf("conectar ao banco (%s, %d tentativa(s)): %w", mc.Addr, attempt, err)
		}
		logger.Warn("db.connect_retry", slog.Int("attempt", attempt), slog.Any("err", err))
		select {
		case <-ctx.Done():
			db.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// permanent: erros de acesso (1044 banco negado, 1045 senha, 1049 banco
// inexistente, 1698 usuário sem senha).
func permanent(err error) bool {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return false
	}
	switch me.Number {
	case 1044, 1045, 1049, 1698:
		return true
	}
	return false
}

// InTx roda fn numa transação: commit se fn devolver nil, rollback se
// devolver erro ou entrar em pânico (o pânico segue adiante).
func InTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	return InTxOpts(ctx, db, nil, fn)
}

// InTxOpts é InTx com opções (ex.: ReadOnly, nível de isolamento).
func InTxOpts(ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("iniciar transação: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}
