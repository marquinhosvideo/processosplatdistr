package httpx

import (
	"context"
	"log/slog"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	loggerKey
	logStateKey
)

// RequestID devolve o ID da requisição corrente (16 hex), ou "" fora de uma
// requisição que passou pelo middleware RequestID.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Logger devolve o logger da requisição, já com request_id. Fora de uma
// requisição, devolve slog.Default().
func Logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func withRequest(ctx context.Context, id string, logger *slog.Logger) context.Context {
	ctx = context.WithValue(ctx, requestIDKey, id)
	return context.WithValue(ctx, loggerKey, logger.With(slog.String("request_id", id)))
}

// logState é preenchido ao longo da requisição e lido pelo AccessLog no fim.
// É um ponteiro no contexto porque camadas internas (sessão) criam cópias do
// *http.Request que o AccessLog não vê.
type logState struct {
	userID int64
}

// SetUserID registra o usuário autenticado para o log de acesso.
func SetUserID(ctx context.Context, userID int64) {
	if st, ok := ctx.Value(logStateKey).(*logState); ok {
		st.userID = userID
	}
}
