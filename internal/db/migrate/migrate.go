// Package migrate é o runner de migrations do schema (SDD §3.2): arquivos
// NNNN_nome.sql embutidos, aplicados em ordem sob GET_LOCK, com checksum de
// cada versão aplicada e marca dirty enquanto um arquivo está em execução.
// DDL no MySQL não é transacional; uma falha no meio deixa a versão dirty, e
// só "server migrate force N" (depois da correção manual) a libera.
package migrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/migrations"
)

// LockName é o lock nomeado (GET_LOCK) que serializa runners de várias
// unidades subindo ao mesmo tempo.
const LockName = "pp:schema_migrate"

// DefaultLockTimeout é a espera máxima pelo lock (§3.2).
const DefaultLockTimeout = 120 * time.Second

// Erros de estado do schema; as mensagens completas dizem o que fazer.
var (
	ErrDirty    = errors.New("schema dirty")
	ErrChecksum = errors.New("checksum divergente")
	ErrBehind   = errors.New("schema atrás do binário")
	ErrLocked   = errors.New("lock de migração indisponível")
)

// IsSchemaProblem informa se err é um dos erros de estado do schema (dirty,
// checksum ou atraso), e não falha de conexão.
func IsSchemaProblem(err error) bool {
	return errors.Is(err, ErrDirty) || errors.Is(err, ErrChecksum) || errors.Is(err, ErrBehind)
}

const createTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INT UNSIGNED NOT NULL,
  name       VARCHAR(200) NOT NULL,
  checksum   BINARY(32)   NOT NULL,
  dirty      BOOLEAN      NOT NULL DEFAULT 1,
  started_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  applied_at DATETIME(3)  NULL,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`

// Migration é um arquivo NNNN_nome.sql.
type Migration struct {
	Version  uint32
	Name     string // "core" em 0001_core.sql
	SQL      string
	Checksum [32]byte // sha256 do conteúdo
}

// File devolve o nome do arquivo de origem.
func (m Migration) File() string { return fmt.Sprintf("%04d_%s.sql", m.Version, m.Name) }

var fileRE = regexp.MustCompile(`^([0-9]{4})_([a-z0-9_]+)\.sql$`)

// Load lê os .sql da raiz de fsys, em ordem de versão. Nome fora do padrão,
// versão 0 ou repetida e arquivo vazio são erro.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("listar migrations: %w", err)
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := fileRE.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migration %q fora do padrão NNNN_nome.sql", e.Name())
		}
		v, _ := strconv.ParseUint(m[1], 10, 32)
		if v == 0 {
			return nil, fmt.Errorf("migration %q: a versão começa em 0001", e.Name())
		}
		content, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("ler %s: %w", e.Name(), err)
		}
		if len(bytes.TrimSpace(content)) == 0 {
			return nil, fmt.Errorf("migration %s vazia", e.Name())
		}
		out = append(out, Migration{Version: uint32(v), Name: m[2], SQL: string(content), Checksum: sha256.Sum256(content)})
	}
	slices.SortFunc(out, func(a, b Migration) int { return int(a.Version) - int(b.Version) })
	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, fmt.Errorf("versão %04d repetida: %s e %s", out[i].Version, out[i-1].File(), out[i].File())
		}
	}
	return out, nil
}

// Embedded devolve as migrations embutidas no binário (pacote migrations).
func Embedded() ([]Migration, error) { return Load(migrations.FS) }

// Open cria a conexão dedicada do runner: multiStatements ligado só aqui e
// uma conexão no máximo. Sem timeout de leitura, porque o GET_LOCK espera até
// 120 s e um ALTER pode demorar; o limite é o contexto.
func Open(mc *mysql.Config) (*sql.DB, error) {
	mc = mc.Clone()
	mc.MultiStatements = true
	mc.ReadTimeout, mc.WriteTimeout = 0, 0
	return db.Open(mc, db.Pool{MaxOpen: 1, MaxIdle: 1, MaxLifetime: time.Hour})
}

// Applied é uma linha de schema_migrations.
type Applied struct {
	Version   uint32
	Name      string
	Checksum  [32]byte
	Dirty     bool
	StartedAt time.Time
	AppliedAt sql.NullTime
}

// Status compara o schema com as migrations do binário.
type Status struct {
	Applied    []Applied
	Pending    []Migration // no binário e não aplicadas
	Unknown    []Applied   // aplicadas e desconhecidas do binário: schema à frente (rollback do app)
	Mismatched []Applied   // aplicadas com checksum diferente do arquivo atual
	Dirty      *Applied
	Current    uint32 // maior versão aplicada e limpa (0 = nenhuma)
	Latest     uint32 // maior versão do binário
}

// Ahead informa se o schema tem versões que o binário não conhece.
func (s Status) Ahead() bool { return len(s.Unknown) > 0 }

// Problem devolve o motivo para não servir com este schema, ou nil: dirty,
// checksum divergente ou migration pendente. Schema à frente não é problema
// (expand/contract permite rodar o app anterior).
func (s Status) Problem() error {
	switch {
	case s.Dirty != nil:
		return fmt.Errorf("%w: a versão %04d (%s) ficou incompleta; corrija o schema à mão e rode \"server migrate force %d\"",
			ErrDirty, s.Dirty.Version, s.Dirty.Name, s.Dirty.Version)
	case len(s.Mismatched) > 0:
		var vs []string
		for _, a := range s.Mismatched {
			vs = append(vs, fmt.Sprintf("%04d (%s)", a.Version, a.Name))
		}
		return fmt.Errorf("%w em %s: o arquivo embutido difere do que foi aplicado; migration mergeada não pode ser editada (crie um novo arquivo)",
			ErrChecksum, strings.Join(vs, ", "))
	case len(s.Pending) > 0:
		return fmt.Errorf("%w: schema na versão %04d e o binário espera %04d (%d pendente(s)); rode \"server migrate up\" ou suba com MIGRATE_ON_START=true",
			ErrBehind, s.Current, s.Latest, len(s.Pending))
	}
	return nil
}

// Runner aplica e inspeciona migrations.
type Runner struct {
	Migrations  []Migration
	Logger      *slog.Logger
	LockTimeout time.Duration // 0 = DefaultLockTimeout
}

// New cria um Runner com as migrations embutidas.
func New(logger *slog.Logger) (*Runner, error) {
	ms, err := Embedded()
	if err != nil {
		return nil, err
	}
	return &Runner{Migrations: ms, Logger: logger}, nil
}

func (r *Runner) logger() *slog.Logger {
	if r.Logger == nil {
		return slog.Default()
	}
	return r.Logger
}

func (r *Runner) latest() uint32 {
	if len(r.Migrations) == 0 {
		return 0
	}
	return r.Migrations[len(r.Migrations)-1].Version
}

// Status lê schema_migrations sem lock. Sem a tabela, tudo está pendente.
func (r *Runner) Status(ctx context.Context, q db.Q) (Status, error) {
	applied, err := readApplied(ctx, q)
	if err != nil {
		return Status{}, err
	}
	return r.compare(applied), nil
}

func (r *Runner) compare(applied []Applied) Status {
	st := Status{Applied: applied, Latest: r.latest()}
	byVersion := map[uint32]Applied{}
	for _, a := range applied {
		byVersion[a.Version] = a
		if a.Dirty && st.Dirty == nil {
			st.Dirty = &a
		}
		if !a.Dirty && a.Version > st.Current {
			st.Current = a.Version
		}
	}
	known := map[uint32]bool{}
	for _, m := range r.Migrations {
		known[m.Version] = true
		a, ok := byVersion[m.Version]
		switch {
		case !ok:
			st.Pending = append(st.Pending, m)
		case !a.Dirty && a.Checksum != m.Checksum:
			st.Mismatched = append(st.Mismatched, a)
		}
	}
	for _, a := range applied {
		if !known[a.Version] {
			st.Unknown = append(st.Unknown, a)
		}
	}
	return st
}

func readApplied(ctx context.Context, q db.Q) ([]Applied, error) {
	rows, err := q.QueryContext(ctx, `SELECT version, name, checksum, dirty, started_at, applied_at FROM schema_migrations ORDER BY version`)
	if db.Code(err) == db.ErrNoSuchTable {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ler schema_migrations: %w", err)
	}
	defer rows.Close()
	var out []Applied
	for rows.Next() {
		var a Applied
		var sum []byte
		if err := rows.Scan(&a.Version, &a.Name, &sum, &a.Dirty, &a.StartedAt, &a.AppliedAt); err != nil {
			return nil, fmt.Errorf("ler schema_migrations: %w", err)
		}
		copy(a.Checksum[:], sum)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ler schema_migrations: %w", err)
	}
	return out, nil
}

// Check é a conferência do serve com MIGRATE_ON_START=false: falha com o
// schema atrás, dirty ou com checksum divergente; à frente, só avisa.
func (r *Runner) Check(ctx context.Context, q db.Q) (Status, error) {
	st, err := r.Status(ctx, q)
	if err != nil {
		return st, err
	}
	if err := st.Problem(); err != nil {
		return st, err
	}
	r.warnAhead(st)
	return st, nil
}

func (r *Runner) warnAhead(st Status) {
	for _, a := range st.Unknown {
		r.logger().Warn("migrate.schema_ahead",
			slog.Int("version", int(a.Version)), slog.String("name", a.Name), slog.Int("binary_latest", int(st.Latest)),
			slog.String("detail", "schema à frente do binário (rollback do app); seguindo"))
	}
}

// Up aplica as migrations pendentes, em ordem, e devolve as aplicadas. dbh
// deve vir de Open (multiStatements); a sessão inteira usa uma só conexão,
// porque o GET_LOCK e o SET FOREIGN_KEY_CHECKS valem por sessão.
func (r *Runner) Up(ctx context.Context, dbh *sql.DB) ([]Migration, error) {
	var done []Migration
	err := r.locked(ctx, dbh, func(conn *sql.Conn) error {
		applied, err := readApplied(ctx, conn)
		if err != nil {
			return err
		}
		st := r.compare(applied)
		if st.Dirty != nil || len(st.Mismatched) > 0 {
			return st.Problem()
		}
		r.warnAhead(st)
		for _, m := range st.Pending {
			if err := r.apply(ctx, conn, m); err != nil {
				return err
			}
			done = append(done, m)
		}
		return nil
	})
	return done, err
}

func (r *Runner) apply(ctx context.Context, conn *sql.Conn, m Migration) error {
	start := time.Now()
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, checksum, dirty) VALUES (?, ?, ?, 1)`,
		m.Version, m.Name, m.Checksum[:]); err != nil {
		return fmt.Errorf("registrar a migration %s: %w", m.File(), err)
	}
	if _, err := conn.ExecContext(ctx, m.SQL); err != nil {
		r.logger().Error("migrate.failed", slog.Int("version", int(m.Version)), slog.String("name", m.Name), slog.Any("err", err))
		return fmt.Errorf("%w: a migration %s falhou e a versão %04d ficou marcada como dirty; corrija o schema à mão e rode \"server migrate force %d\": %w",
			ErrDirty, m.File(), m.Version, m.Version, err)
	}
	if _, err := conn.ExecContext(ctx,
		`UPDATE schema_migrations SET dirty = 0, applied_at = NOW(3) WHERE version = ?`, m.Version); err != nil {
		return fmt.Errorf("concluir a migration %s: %w", m.File(), err)
	}
	r.logger().Info("migrate.applied", slog.Int("version", int(m.Version)), slog.String("name", m.Name),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()))
	return nil
}

// Force marca version como aplicada e limpa, com o checksum do arquivo atual.
// É para depois de uma correção manual de uma migration que falhou no meio.
func (r *Runner) Force(ctx context.Context, dbh *sql.DB, version uint32) error {
	i := slices.IndexFunc(r.Migrations, func(m Migration) bool { return m.Version == version })
	if i < 0 {
		return fmt.Errorf("a versão %04d não existe neste binário (última: %04d)", version, r.latest())
	}
	m := r.Migrations[i]
	return r.locked(ctx, dbh, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, `
INSERT INTO schema_migrations (version, name, checksum, dirty, applied_at) VALUES (?, ?, ?, 0, NOW(3))
ON DUPLICATE KEY UPDATE name = VALUES(name), checksum = VALUES(checksum), dirty = 0, applied_at = NOW(3)`,
			m.Version, m.Name, m.Checksum[:]); err != nil {
			return fmt.Errorf("forçar a versão %04d: %w", m.Version, err)
		}
		r.logger().Warn("migrate.forced", slog.Int("version", int(m.Version)), slog.String("name", m.Name))
		return nil
	})
}

// locked roda fn numa conexão fixa, com o GET_LOCK obtido e a tabela
// schema_migrations criada.
func (r *Runner) locked(ctx context.Context, dbh *sql.DB, fn func(conn *sql.Conn) error) error {
	conn, err := dbh.Conn(ctx)
	if err != nil {
		return fmt.Errorf("conexão da migração: %w", err)
	}
	defer conn.Close()

	timeout := r.LockTimeout
	if timeout <= 0 {
		timeout = DefaultLockTimeout
	}
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, LockName, int(timeout.Seconds())).Scan(&got); err != nil {
		return fmt.Errorf("obter o lock %s: %w", LockName, err)
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("%w: %s não foi obtido em %s (outra unidade migrando?)", ErrLocked, LockName, timeout)
	}
	defer func() {
		// Mesmo com ctx cancelado: sem o RELEASE, o lock só cai com a sessão.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(rctx, `DO RELEASE_LOCK(?)`, LockName); err != nil {
			r.logger().Warn("migrate.unlock_failed", slog.Any("err", err))
		}
	}()

	if _, err := conn.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}
	return fn(conn)
}

// UpConfig abre a conexão dedicada a partir de mc, aplica as migrations
// embutidas e fecha a conexão (é o migrate.Up(ctx, dsn) da §3.2).
func UpConfig(ctx context.Context, mc *mysql.Config, logger *slog.Logger) ([]Migration, error) {
	r, err := New(logger)
	if err != nil {
		return nil, err
	}
	dbh, err := Open(mc)
	if err != nil {
		return nil, err
	}
	defer dbh.Close()
	return r.Up(ctx, dbh)
}
