// Package audit grava a trilha de auditoria (audit_log, RNF-07) e o log de
// acesso a dados sensíveis (access_log, RNF-06), conforme a §3.10 do SDD.
//
// Cada caso de uso chama Record dentro da própria transação, com structs de
// snapshot antes e depois. Todo campo de snapshot leva a tag audit:"plain"
// (gravado com o valor) ou audit:"redacted" (gravado só como
// {"changed":true}): PII de candidato, dados restritos e o conteúdo do Nine
// Box nunca entram no audit_log. Campo sem tag é tratado como redacted, e um
// teste percorre o código e falha se algum snapshot tiver campo sem tag.
// Convenção: tipos de snapshot terminam em "Snapshot".
//
// A tabela é append-only: o pacote só faz INSERT.
package audit

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/internal/httpx"
)

// Tipos de titular (subject_type).
const (
	SubjectCandidate = "candidate"
	SubjectPerson    = "person"
)

// Subject é o titular dos dados afetados ou expostos.
type Subject struct {
	Type string
	ID   int64
}

// Candidate e Person montam o titular.
func Candidate(id int64) Subject { return Subject{Type: SubjectCandidate, ID: id} }
func Person(id int64) Subject    { return Subject{Type: SubjectPerson, ID: id} }

func (s Subject) valid() bool {
	return (s.Type == SubjectCandidate || s.Type == SubjectPerson) && s.ID > 0
}

// Actor é quem age: o usuário da sessão e a sua gerência. Sem Actor no
// contexto, o ator é o sistema (job, bootstrap) e actor_user_id fica NULL.
type Actor struct {
	UserID    int64
	OrgUnitID int64
}

type actorKey struct{}

// WithActor guarda o ator no contexto; a sessão autenticada faz isso em
// cada requisição.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom devolve o ator do contexto.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// Entry é uma escrita auditada.
type Entry struct {
	Action     string   // ex.: candidate.transition, quarter.close
	EntityType string   // ex.: candidate
	EntityID   int64    // 0 = sem id
	Subject    *Subject // titular afetado, se houver
	Before     any      // snapshot (struct ou ponteiro) antes; nil na criação
	After      any      // snapshot depois; nil na exclusão
	Reason     string   // só ações administrativas (override, anonimização)
	OrgUnitID  int64    // 0 = a gerência do ator
}

// ErrInvalid indica uma Entry ou um acesso mal formado (erro de programação).
var ErrInvalid = errors.New("auditoria inválida")

// Limites das colunas.
const (
	maxAction     = 64
	maxEntityType = 40
	maxReason     = 1000
	maxAccessAct  = 48
)

// Record grava e em audit_log usando q, normalmente a transação do caso de
// uso: se a gravação falhar, a escrita inteira volta atrás.
func Record(ctx context.Context, q db.Q, e Entry) error {
	switch {
	case e.Action == "" || len(e.Action) > maxAction:
		return fmt.Errorf("%w: action %q", ErrInvalid, e.Action)
	case e.EntityType == "" || len(e.EntityType) > maxEntityType:
		return fmt.Errorf("%w: entity_type %q", ErrInvalid, e.EntityType)
	case utf8.RuneCountInString(e.Reason) > maxReason:
		return fmt.Errorf("%w: reason com mais de %d caracteres", ErrInvalid, maxReason)
	case e.Subject != nil && !e.Subject.valid():
		return fmt.Errorf("%w: titular %+v", ErrInvalid, *e.Subject)
	}
	before, after, err := Diff(e.Before, e.After)
	if err != nil {
		return err
	}

	actor, _ := ActorFrom(ctx)
	orgUnit := e.OrgUnitID
	if orgUnit == 0 {
		orgUnit = actor.OrgUnitID
	}
	var subjectType, subjectID any
	if e.Subject != nil {
		subjectType, subjectID = e.Subject.Type, e.Subject.ID
	}
	_, err = q.ExecContext(ctx, `INSERT INTO audit_log
  (org_unit_id, actor_user_id, action, entity_type, entity_id, subject_type, subject_id, before_json, after_json, reason, request_id, ip)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullID(orgUnit), nullID(actor.UserID), e.Action, e.EntityType, nullID(e.EntityID), subjectType, subjectID,
		nullJSON(before), nullJSON(after), nullStr(e.Reason), nullStr(httpx.RequestID(ctx)), nullStr(httpx.ClientIP(ctx)))
	if err != nil {
		return fmt.Errorf("gravar auditoria %s: %w", e.Action, err)
	}
	return nil
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullJSON(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}
