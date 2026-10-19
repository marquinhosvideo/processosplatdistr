package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/marquinhosvideo/processosplatdistr/internal/db"
	"github.com/marquinhosvideo/processosplatdistr/internal/httpx"
)

// Resource é o objeto lido (ex.: ninebox_entry 12, quarter 3).
type Resource struct {
	Type string
	ID   int64 // 0 = sem id (ex.: uma lista)
}

// accessBatch limita as linhas por INSERT (uma matriz do Nine Box expõe
// centenas de titulares).
const accessBatch = 500

// Access registra em access_log uma linha por titular exposto a userID
// (RNF-06). É chamado dentro do método de leitura sensível, antes de
// serializar a resposta, e é fail-closed: se devolver erro, quem chama
// responde 500 e o dado não sai. Titulares repetidos contam uma vez; sem
// titular, nada é gravado.
func Access(ctx context.Context, q db.Q, userID int64, action string, res Resource, subjects ...Subject) error {
	switch {
	case userID <= 0:
		return fmt.Errorf("%w: access_log sem usuário", ErrInvalid)
	case action == "" || len(action) > maxAccessAct:
		return fmt.Errorf("%w: action %q", ErrInvalid, action)
	case res.Type == "" || len(res.Type) > maxEntityType:
		return fmt.Errorf("%w: resource_type %q", ErrInvalid, res.Type)
	}
	seen := make(map[Subject]bool, len(subjects))
	unique := make([]Subject, 0, len(subjects))
	for _, s := range subjects {
		if !s.valid() {
			return fmt.Errorf("%w: titular %+v", ErrInvalid, s)
		}
		if !seen[s] {
			seen[s] = true
			unique = append(unique, s)
		}
	}
	reqID, ip := nullStr(httpx.RequestID(ctx)), nullStr(httpx.ClientIP(ctx))
	for start := 0; start < len(unique); start += accessBatch {
		chunk := unique[start:min(start+accessBatch, len(unique))]
		args := make([]any, 0, len(chunk)*8)
		for _, s := range chunk {
			args = append(args, userID, action, res.Type, nullID(res.ID), s.Type, s.ID, reqID, ip)
		}
		query := `INSERT INTO access_log (user_id, action, resource_type, resource_id, subject_type, subject_id, request_id, ip) VALUES ` +
			strings.TrimSuffix(strings.Repeat("(?, ?, ?, ?, ?, ?, ?, ?), ", len(chunk)), ", ")
		if _, err := q.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("gravar access_log %s: %w", action, err)
		}
	}
	return nil
}
