package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// ensureOrgUnit garante a gerência DEFAULT_ORG_UNIT_* e devolve o id. É
// idempotente e seguro com várias unidades subindo juntas (UNIQUE em code).
// Uma gerência existente não é renomeada pela variável: só registra aviso.
func ensureOrgUnit(ctx context.Context, h *sql.DB, code, name string, logger *slog.Logger) (int64, error) {
	res, err := h.ExecContext(ctx,
		`INSERT INTO org_units (code, name) VALUES (?, ?) ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`, code, name)
	if err != nil {
		return 0, fmt.Errorf("bootstrap da gerência %s: %w", code, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("bootstrap da gerência %s: %w", code, err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		logger.Info("bootstrap.org_unit_created", slog.String("code", code), slog.Int64("id", id))
		return id, nil
	}
	var current string
	if err := h.QueryRowContext(ctx, `SELECT name FROM org_units WHERE id = ?`, id).Scan(&current); err != nil {
		return 0, fmt.Errorf("bootstrap da gerência %s: %w", code, err)
	}
	if current != name {
		logger.Warn("bootstrap.org_unit_name_differs", slog.String("code", code),
			slog.String("detail", "DEFAULT_ORG_UNIT_NAME difere do nome gravado; o nome do banco foi mantido"))
	}
	return id, nil
}
