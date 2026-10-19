package migrate

import (
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadOrdersAndChecksums(t *testing.T) {
	fsys := fstest.MapFS{
		"0002_org.sql":   {Data: []byte("CREATE TABLE b (id INT PRIMARY KEY);")},
		"0001_core.sql":  {Data: []byte("CREATE TABLE a (id INT PRIMARY KEY);")},
		"embed.go":       {Data: []byte("package migrations")},
		"README.md":      {Data: []byte("x")},
		"sub/0003_x.sql": {Data: []byte("SELECT 1")},
	}
	ms, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].Version != 1 || ms[0].Name != "core" || ms[1].Version != 2 || ms[1].File() != "0002_org.sql" {
		t.Fatalf("migrations = %+v", ms)
	}
	if ms[0].Checksum != sha256.Sum256([]byte("CREATE TABLE a (id INT PRIMARY KEY);")) {
		t.Error("checksum não é o sha256 do conteúdo")
	}
}

func TestLoadRejects(t *testing.T) {
	tests := map[string]fstest.MapFS{
		"nome fora do padrão": {"1_core.sql": {Data: []byte("x")}},
		"maiúsculas":          {"0001_Core.sql": {Data: []byte("x")}},
		"versão zero":         {"0000_core.sql": {Data: []byte("x")}},
		"versão repetida":     {"0001_a.sql": {Data: []byte("x")}, "0001_b.sql": {Data: []byte("y")}},
		"arquivo vazio":       {"0001_core.sql": {Data: []byte(" \n")}},
	}
	for name, fsys := range tests {
		if ms, err := Load(fsys); err == nil {
			t.Errorf("%s: aceito %+v", name, ms)
		}
	}
}

func TestEmbedded(t *testing.T) {
	ms, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 || ms[0].Version != 1 || ms[0].Name != "core" {
		t.Fatalf("migrations embutidas = %+v", ms)
	}
	for i, m := range ms {
		if m.Version != uint32(i+1) {
			t.Errorf("buraco na numeração: %s na posição %d", m.File(), i)
		}
	}
	// 0001 é o DDL da §3.3: abre e fecha com FOREIGN_KEY_CHECKS e só cria.
	core := ms[0].SQL
	if !strings.HasPrefix(core, "SET FOREIGN_KEY_CHECKS = 0;") || !strings.HasSuffix(strings.TrimSpace(core), "SET FOREIGN_KEY_CHECKS = 1;") {
		t.Error("0001_core.sql deve abrir e fechar com FOREIGN_KEY_CHECKS")
	}
	for _, table := range []string{"org_units", "job_titles", "teams", "people", "users", "sessions", "app_settings",
		"audit_log", "access_log", "job_runs", "notifications", "user_activity_days"} {
		if !strings.Contains(core, "CREATE TABLE IF NOT EXISTS "+table+" (") {
			t.Errorf("0001_core.sql sem a tabela %s", table)
		}
	}
}

func TestCompare(t *testing.T) {
	m1 := Migration{Version: 1, Name: "core", Checksum: sha256.Sum256([]byte("1"))}
	m2 := Migration{Version: 2, Name: "org", Checksum: sha256.Sum256([]byte("2"))}
	r := &Runner{Migrations: []Migration{m1, m2}}

	st := r.compare(nil)
	if st.Current != 0 || st.Latest != 2 || len(st.Pending) != 2 || !errors.Is(st.Problem(), ErrBehind) {
		t.Errorf("banco vazio: %+v %v", st, st.Problem())
	}

	ok1 := Applied{Version: 1, Name: "core", Checksum: m1.Checksum}
	ok2 := Applied{Version: 2, Name: "org", Checksum: m2.Checksum}
	if st := r.compare([]Applied{ok1, ok2}); st.Problem() != nil || st.Current != 2 || st.Ahead() {
		t.Errorf("em dia: %+v %v", st, st.Problem())
	}

	ahead := Applied{Version: 3, Name: "novo"}
	if st := r.compare([]Applied{ok1, ok2, ahead}); st.Problem() != nil || !st.Ahead() || st.Current != 3 {
		t.Errorf("à frente deveria só avisar: %+v %v", st, st.Problem())
	}

	dirty := Applied{Version: 2, Name: "org", Checksum: m2.Checksum, Dirty: true}
	st = r.compare([]Applied{ok1, dirty})
	if err := st.Problem(); !errors.Is(err, ErrDirty) || !strings.Contains(err.Error(), "server migrate force 2") || st.Current != 1 {
		t.Errorf("dirty: %v", err)
	}

	bad := ok1
	bad.Checksum = sha256.Sum256([]byte("editado"))
	if err := r.compare([]Applied{bad, ok2}).Problem(); !errors.Is(err, ErrChecksum) || !strings.Contains(err.Error(), "0001 (core)") {
		t.Errorf("checksum: %v", err)
	}
}
