package audit

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type candidateSnapshot struct {
	ID        int64     `json:"id" audit:"plain"`
	Status    string    `json:"status" audit:"plain"`
	FullName  string    `json:"fullName" audit:"redacted"`
	Email     string    `json:"email" audit:"redacted"`
	OwnerID   int64     `audit:"plain"`
	Updated   time.Time `json:"updatedAt" audit:"plain"`
	Skills    []string  `json:"skills" audit:"plain"`
	Secret    string    `json:"-" audit:"redacted"`
	untracked string
}

func TestDiffUpdateOnlyChangedFields(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	before := candidateSnapshot{ID: 7, Status: "talent_pool", FullName: "Maria Silva", Email: "maria@x.com", OwnerID: 1, Updated: ts, Skills: []string{"go"}}
	after := before
	after.Status = "in_process"
	after.FullName = "Maria S. Silva"
	after.Skills = []string{"go", "kafka"}
	after.Secret = "não sai"
	after.untracked = "x"

	b, a, err := Diff(before, &after)
	if err != nil {
		t.Fatal(err)
	}
	wantB := `{"fullName":{"changed":true},"skills":["go"],"status":"talent_pool"}`
	wantA := `{"fullName":{"changed":true},"skills":["go","kafka"],"status":"in_process"}`
	if string(b) != wantB || string(a) != wantA {
		t.Errorf("before = %s\nafter  = %s", b, a)
	}
}

func TestDiffCreateAndDelete(t *testing.T) {
	s := candidateSnapshot{ID: 3, Status: "talent_pool", FullName: "João", OwnerID: 9}
	b, a, err := Diff(nil, s)
	if err != nil {
		t.Fatal(err)
	}
	// Na criação, todos os plain; redacted só se preenchido, e sem o valor.
	// Sem tag json, o nome é o do campo com a inicial minúscula.
	want := `{"fullName":{"changed":true},"id":3,"ownerID":9,"skills":null,"status":"talent_pool","updatedAt":"0001-01-01T00:00:00Z"}`
	if b != nil || string(a) != want {
		t.Errorf("criação: before=%s after=%s", b, a)
	}
	b, a, err = Diff(&s, (*candidateSnapshot)(nil))
	if err != nil || a != nil || string(b) != want {
		t.Errorf("exclusão: before=%s after=%s err=%v", b, a, err)
	}
}

func TestDiffNoChangeIsNull(t *testing.T) {
	s := candidateSnapshot{ID: 1, Status: "hired"}
	b, a, err := Diff(s, s)
	if err != nil || b != nil || a != nil {
		t.Errorf("sem mudança: %s %s %v", b, a, err)
	}
	if b, a, err := Diff(nil, nil); err != nil || b != nil || a != nil {
		t.Errorf("nil: %s %s %v", b, a, err)
	}
}

type untaggedSnapshot struct {
	Status string `audit:"plain"`
	Name   string // sem tag: redacted em runtime
	Typo   string `audit:"plian"`
}

func TestUntaggedFieldIsRedactedAtRuntime(t *testing.T) {
	b, a, err := Diff(untaggedSnapshot{Status: "a", Name: "Ana", Typo: "x"}, untaggedSnapshot{Status: "b", Name: "Bia", Typo: "y"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"name":{"changed":true},"status":"a","typo":{"changed":true}}` || string(a) != `{"name":{"changed":true},"status":"b","typo":{"changed":true}}` {
		t.Errorf("before=%s after=%s", b, a)
	}
	if got := CheckSnapshot(reflect.TypeOf(untaggedSnapshot{})); len(got) != 2 {
		t.Errorf("CheckSnapshot = %v", got)
	}
	if got := CheckSnapshot(reflect.TypeOf(&candidateSnapshot{})); len(got) != 0 {
		t.Errorf("CheckSnapshot de snapshot correto = %v", got)
	}
}

func TestDiffRejectsNonStruct(t *testing.T) {
	if _, _, err := Diff(map[string]string{"a": "b"}, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v", err)
	}
}
