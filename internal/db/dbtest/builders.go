package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/marquinhosvideo/processosplatdistr/internal/db"
)

// Builders das linhas básicas do núcleo (0001). Campo zero recebe um valor
// único e válido; os módulos acrescentam os seus builders nos próprios testes.

var seq atomic.Int64

func next() int64 { return seq.Add(1) }

func insert(t testing.TB, q db.Q, query string, args ...any) int64 {
	t.Helper()
	res, err := q.ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("dbtest: %v\n%s", err, query)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func nullID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// OrgUnit é uma gerência.
type OrgUnit struct {
	Code, Name string
}

// InsertOrgUnit grava a gerência e devolve o id.
func InsertOrgUnit(t testing.TB, q db.Q, o OrgUnit) int64 {
	t.Helper()
	n := next()
	if o.Code == "" {
		o.Code = fmt.Sprintf("OU%d", n)
	}
	if o.Name == "" {
		o.Name = "Gerência " + o.Code
	}
	return insert(t, q, `INSERT INTO org_units (code, name) VALUES (?, ?)`, o.Code, o.Name)
}

// JobTitle é um cargo.
type JobTitle struct {
	Code, Name string
}

// InsertJobTitle grava o cargo e devolve o id.
func InsertJobTitle(t testing.TB, q db.Q, j JobTitle) int64 {
	t.Helper()
	n := next()
	if j.Code == "" {
		j.Code = fmt.Sprintf("JT%d", n)
	}
	if j.Name == "" {
		j.Name = "Cargo " + j.Code
	}
	return insert(t, q, `INSERT INTO job_titles (code, name) VALUES (?, ?)`, j.Code, j.Name)
}

// Team é um time; OrgUnitID é obrigatório.
type Team struct {
	OrgUnitID       int64
	Code, Name      string
	ManagerPersonID int64
}

// InsertTeam grava o time e devolve o id.
func InsertTeam(t testing.TB, q db.Q, tm Team) int64 {
	t.Helper()
	if tm.OrgUnitID == 0 {
		t.Fatal("dbtest.InsertTeam: OrgUnitID obrigatório")
	}
	n := next()
	if tm.Code == "" {
		tm.Code = fmt.Sprintf("T%d", n)
	}
	if tm.Name == "" {
		tm.Name = "Time " + tm.Code
	}
	return insert(t, q, `INSERT INTO teams (org_unit_id, code, name, manager_person_id) VALUES (?, ?, ?, ?)`,
		tm.OrgUnitID, tm.Code, tm.Name, nullID(tm.ManagerPersonID))
}

// Person é uma pessoa do ERP; OrgUnitID é obrigatório.
type Person struct {
	OrgUnitID      int64
	TeamID         int64
	EmployeeNumber string
	FullName       string
	Email          string
	JobTitleID     int64
	ERPManagerID   int64
}

// InsertPerson grava a pessoa e devolve o id.
func InsertPerson(t testing.TB, q db.Q, p Person) int64 {
	t.Helper()
	if p.OrgUnitID == 0 {
		t.Fatal("dbtest.InsertPerson: OrgUnitID obrigatório")
	}
	n := next()
	if p.EmployeeNumber == "" {
		p.EmployeeNumber = fmt.Sprintf("M%06d", n)
	}
	if p.FullName == "" {
		p.FullName = fmt.Sprintf("Pessoa %d", n)
	}
	if p.Email == "" {
		p.Email = fmt.Sprintf("pessoa%d@exemplo.com.br", n)
	}
	return insert(t, q, `INSERT INTO people (org_unit_id, team_id, employee_number, full_name, email, email_norm, job_title_id, erp_manager_person_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.OrgUnitID, nullID(p.TeamID), p.EmployeeNumber, p.FullName, p.Email, strings.ToLower(strings.TrimSpace(p.Email)),
		nullID(p.JobTitleID), nullID(p.ERPManagerID))
}

// User é um usuário do IdP; Roles é o SET de last_roles ("manager,head").
type User struct {
	Issuer, Subject string
	Email, Name     string
	PersonID        int64
	Roles           string
}

// InsertUser grava o usuário e devolve o id.
func InsertUser(t testing.TB, q db.Q, u User) int64 {
	t.Helper()
	n := next()
	if u.Issuer == "" {
		u.Issuer = "http://localhost:9000"
	}
	if u.Subject == "" {
		u.Subject = fmt.Sprintf("sub-%d", n)
	}
	if u.Email == "" {
		u.Email = fmt.Sprintf("usuario%d@exemplo.com.br", n)
	}
	if u.Name == "" {
		u.Name = fmt.Sprintf("Usuário %d", n)
	}
	if u.Roles == "" {
		u.Roles = "manager"
	}
	link := ""
	if u.PersonID != 0 {
		link = "manual"
	}
	return insert(t, q, `INSERT INTO users (oidc_issuer, oidc_subject, email, email_norm, display_name, person_id, person_link, last_roles)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Issuer, u.Subject, u.Email, strings.ToLower(strings.TrimSpace(u.Email)), u.Name, nullID(u.PersonID), nullStr(link), u.Roles)
}
