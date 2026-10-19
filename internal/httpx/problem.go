package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// ProblemContentType é o media type dos erros (RFC 9457).
const ProblemContentType = "application/problem+json"

// Membros do problem que extensões não podem sobrescrever.
var reservedMembers = map[string]bool{
	"type": true, "title": true, "status": true, "code": true,
	"detail": true, "errors": true, "requestId": true, "instance": true,
}

// HandlerFunc é um handler que devolve erro em vez de escrevê-lo. O erro é
// renderizado por WriteError, o único ponto que produz problem+json.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := f(w, r); err != nil {
		WriteError(w, r, err)
	}
}

// WriteError responde err como application/problem+json. *Error usa o próprio
// status e código; *http.MaxBytesError vira 413; qualquer outro erro vira 500
// sem detalhe, só com o requestId, e a causa vai para o log.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	e := asError(err)
	logError(r, e)
	writeProblem(w, r, e)
}

func writeProblem(w http.ResponseWriter, r *http.Request, e *Error) {
	body := map[string]any{
		"type":   "urn:pp:error:" + e.Code,
		"title":  codes[e.Code].title,
		"status": e.Status,
		"code":   e.Code,
	}
	if id := RequestID(r.Context()); id != "" {
		body["requestId"] = id
	}
	// O 500 nunca expõe detalhe nem extensões (§4.1); o 503 explica o que
	// está indisponível, com detalhe escrito para isso (a causa fica no log).
	if e.Status != http.StatusInternalServerError {
		if e.Detail != "" {
			body["detail"] = e.Detail
		}
		if len(e.Errors) > 0 {
			body["errors"] = e.Errors
		}
		for k, v := range e.Ext {
			if !reservedMembers[k] {
				body[k] = v
			}
		}
	}

	h := w.Header()
	h.Set("Content-Type", ProblemContentType)
	h.Set("Cache-Control", "no-store")
	h.Del("Content-Length")
	h.Del("Content-Disposition")
	h.Del("ETag")
	w.WriteHeader(e.Status)
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		Logger(r.Context()).Debug("http.write_failed", slog.Any("err", err))
	}
}

func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		if _, ok := codes[e.Code]; ok && e.Status == codes[e.Code].status {
			return e
		}
		// Status e código inconsistentes são bug; não arrisca expor o detalhe.
		return Internal(err)
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return E(CodePayloadTooLarge).WithCause(err)
	}
	return Internal(err)
}

func logError(r *http.Request, e *Error) {
	logger := Logger(r.Context())
	switch {
	case e.Status >= http.StatusInternalServerError:
		// Requisição abandonada pelo cliente não é falha do servidor.
		if r.Context().Err() != nil {
			logger.Info("http.canceled", slog.String("code", e.Code), slog.Any("err", e.Cause))
			return
		}
		logger.Error("http.error", slog.String("code", e.Code), slog.Any("err", e.Cause))
	case e.Cause != nil:
		logger.Debug("http.client_error", slog.String("code", e.Code), slog.Any("err", e.Cause))
	}
}
