package db_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/internal/db/dbtest"
)

func TestMain(m *testing.M) { dbtest.Main(m) }

func TestSessionParameters(t *testing.T) {
	h := dbtest.DB(t)
	var tz, mode, coll, charset string
	var maxPacket int64
	if err := h.QueryRow(`SELECT @@session.time_zone, @@session.sql_mode, @@collation_connection, @@character_set_connection, @@max_allowed_packet`).
		Scan(&tz, &mode, &coll, &charset, &maxPacket); err != nil {
		t.Fatal(err)
	}
	if tz != db.TimeZone || coll != db.Collation || charset != "utf8mb4" {
		t.Errorf("time_zone=%q collation=%q charset=%q", tz, coll, charset)
	}
	// O servidor devolve os modos em outra ordem; confere um a um.
	for _, want := range []string{"STRICT_TRANS_TABLES", "NO_ZERO_IN_DATE", "NO_ZERO_DATE", "ERROR_FOR_DIVISION_BY_ZERO", "NO_ENGINE_SUBSTITUTION", "ONLY_FULL_GROUP_BY"} {
		if !slices.Contains(strings.Split(mode, ","), want) {
			t.Errorf("sql_mode %q sem %s", mode, want)
		}
	}
	if maxPacket <= 0 {
		t.Errorf("max_allowed_packet = %d", maxPacket)
	}

	// DATETIME(3) volta como time.Time em UTC (parseTime + loc=UTC).
	var now time.Time
	if err := h.QueryRow(`SELECT NOW(3)`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	if now.Location() != time.UTC || time.Since(now).Abs() > time.Minute {
		t.Errorf("NOW(3) = %v (%v)", now, now.Location())
	}
	// Busca sem acento e sem caixa (utf8mb4_0900_ai_ci).
	var eq bool
	if err := h.QueryRow(`SELECT 'João' = 'joao'`).Scan(&eq); err != nil || !eq {
		t.Errorf("comparação sem acento = %v %v", eq, err)
	}
	// O pool da aplicação não aceita vários comandos num Exec.
	if _, err := h.Exec(`SELECT 1; SELECT 2`); err == nil {
		t.Error("multiStatements ligado no pool da aplicação")
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestCheckVersionAndPacket(t *testing.T) {
	h := dbtest.DB(t)
	if v, err := db.CheckVersion(t.Context(), h); err != nil || v == "" {
		t.Errorf("CheckVersion = %q %v", v, err)
	}
	if err := db.CheckMaxAllowedPacket(t.Context(), h, 1<<20); err != nil {
		t.Errorf("1 MB: %v", err)
	}
	if err := db.CheckMaxAllowedPacket(t.Context(), h, 1<<40); err == nil {
		t.Error("1 TB deveria falhar")
	}
}

func TestInTx(t *testing.T) {
	dbtest.Reset(t)
	h := dbtest.DB(t)
	ctx := t.Context()
	count := func() int {
		var n int
		if err := h.QueryRow(`SELECT COUNT(*) FROM org_units`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if err := db.InTx(ctx, h, func(tx *sql.Tx) error {
		dbtest.InsertOrgUnit(t, tx, dbtest.OrgUnit{})
		return nil
	}); err != nil || count() != 1 {
		t.Fatalf("commit: %v, %d linhas", err, count())
	}

	boom := errors.New("regra violada")
	if err := db.InTx(ctx, h, func(tx *sql.Tx) error {
		dbtest.InsertOrgUnit(t, tx, dbtest.OrgUnit{})
		return boom
	}); !errors.Is(err, boom) || count() != 1 {
		t.Fatalf("rollback por erro: %v, %d linhas", err, count())
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("pânico engolido pelo InTx")
			}
		}()
		_ = db.InTx(ctx, h, func(tx *sql.Tx) error {
			dbtest.InsertOrgUnit(t, tx, dbtest.OrgUnit{})
			panic("falha")
		})
	}()
	if count() != 1 {
		t.Errorf("rollback por pânico: %d linhas", count())
	}
}

func TestErrorHelpersAgainstMySQL(t *testing.T) {
	dbtest.Reset(t)
	h := dbtest.DB(t)
	ctx := t.Context()
	ou := dbtest.InsertOrgUnit(t, h, dbtest.OrgUnit{Code: "DUP"})

	_, err := h.ExecContext(ctx, `INSERT INTO org_units (code, name) VALUES ('DUP', 'x')`)
	if !db.IsDuplicate(err) {
		t.Fatalf("duplicidade: %v", err)
	}
	if key, _ := db.DuplicateKey(err); key != "uq_org_units_code" {
		t.Errorf("DuplicateKey = %q", key)
	}

	dbtest.InsertPerson(t, h, dbtest.Person{OrgUnitID: ou, Email: "Ana@Exemplo.com.br"})
	_, err = h.ExecContext(ctx, `INSERT INTO people (org_unit_id, employee_number, full_name, email, email_norm) VALUES (?, 'X1', 'Outra', 'ana@exemplo.com.br', 'ana@exemplo.com.br')`, ou)
	if key, ok := db.DuplicateKey(err); !ok || key != "uq_people_email" {
		t.Errorf("e-mail duplicado: %q %v (%v)", key, ok, err)
	}

	if _, err := h.ExecContext(ctx, `INSERT INTO teams (org_unit_id, code, name) VALUES (999999, 'T', 'T')`); !db.IsFKViolation(err) {
		t.Errorf("FK sem pai: %v", err)
	}
	dbtest.InsertTeam(t, h, dbtest.Team{OrgUnitID: ou})
	if _, err := h.ExecContext(ctx, `DELETE FROM org_units WHERE id = ?`, ou); !db.IsFKViolation(err) {
		t.Errorf("FK com filhos: %v", err)
	}
	if _, err := h.ExecContext(ctx, `INSERT INTO job_titles (code, name, source) VALUES ('C', 'C', 'planilha')`); !db.IsCheckViolation(err) {
		t.Errorf("CHECK: %v", err)
	}
}

func TestLockNowait(t *testing.T) {
	dbtest.Reset(t)
	h := dbtest.DB(t)
	ctx := t.Context()
	ou := dbtest.InsertOrgUnit(t, h, dbtest.OrgUnit{})

	holder, err := h.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if _, err := holder.ExecContext(ctx, `SELECT id FROM org_units WHERE id = ? FOR UPDATE`, ou); err != nil {
		t.Fatal(err)
	}

	err = db.InTx(ctx, h, func(tx *sql.Tx) error {
		var id int64
		return tx.QueryRowContext(ctx, `SELECT id FROM org_units WHERE id = ? FOR UPDATE NOWAIT`, ou).Scan(&id)
	})
	if !db.IsLockNowait(err) {
		t.Fatalf("NOWAIT = %v", err)
	}
}

func TestConnectRetriesUntilDeadline(t *testing.T) {
	mc := dbtest.Config(t)
	mc.Addr = "127.0.0.1:1" // nada escuta aqui
	start := time.Now()
	_, err := db.Connect(context.Background(), mc, db.DefaultPool, 1500*time.Millisecond, discard())
	if err == nil || time.Since(start) < time.Second {
		t.Fatalf("Connect = %v depois de %v", err, time.Since(start))
	}

	// Senha errada é erro do servidor: falha sem esperar.
	mc = dbtest.Config(t)
	mc.Passwd = "errada"
	start = time.Now()
	if _, err := db.Connect(context.Background(), mc, db.DefaultPool, 10*time.Second, discard()); err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("Connect com senha errada = %v depois de %v", err, time.Since(start))
	}
}
