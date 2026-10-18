package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// MaxJSONBytes limita o corpo das escritas JSON (§4.1).
const MaxJSONBytes = 1 << 20

// WriteJSON responde v como JSON com o status dado.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		WriteError(w, r, Internal(fmt.Errorf("serializar resposta: %w", err)))
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(b)+1))
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		Logger(r.Context()).Debug("http.write_failed", slog.Any("err", err))
	}
}

// IsJSON informa se o Content-Type é application/json (parâmetros, como
// charset, são aceitos).
func IsJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && mt == "application/json"
}

// DecodeJSON lê um único objeto JSON do corpo para dst. Campos desconhecidos
// são recusados (contra mass assignment). Erros já saem como *Error: 415 sem
// Content-Type JSON, 413 acima de MaxJSONBytes e 422 para o resto.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if !IsJSON(r.Header.Get("Content-Type")) {
		return E(CodeUnsupportedMediaType)
	}
	body := http.MaxBytesReader(w, r.Body, MaxJSONBytes)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return E(CodePayloadTooLarge).WithCause(err)
		}
		return E(CodeValidationFailed).WithDetail("O corpo deve conter um único objeto JSON.")
	}
	return nil
}

func decodeError(err error) *Error {
	var (
		mbe       *http.MaxBytesError
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &mbe):
		return E(CodePayloadTooLarge).WithCause(err)
	case errors.Is(err, io.EOF):
		return E(CodeValidationFailed).WithDetail("O corpo da requisição está vazio.")
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return E(CodeValidationFailed).WithDetail("O corpo não é um JSON válido.").WithCause(err)
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return E(CodeValidationFailed).WithDetail("O corpo deve ser um objeto JSON.").WithCause(err)
		}
		return Validation(map[string]string{typeErr.Field: "tipo inválido"}).WithCause(err)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// O encoding/json não exporta um tipo para este erro.
		field, uerr := strconv.Unquote(strings.TrimPrefix(err.Error(), "json: unknown field "))
		if uerr != nil {
			field = "_"
		}
		return Validation(map[string]string{field: "campo desconhecido"}).WithCause(err)
	default:
		return E(CodeValidationFailed).WithDetail("O corpo não é um JSON válido.").WithCause(err)
	}
}

// unsafeMethod indica os métodos que alteram estado (CSRF, JSON obrigatório).
func unsafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	}
	return true
}

// RequireJSON exige Content-Type application/json em toda escrita (415) e
// limita o corpo a MaxJSONBytes (413). Rotas de upload usam outro middleware.
// Exigir o JSON também impede que um formulário de outro site chegue à API
// como "simple request".
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unsafeMethod(r.Method) {
			if !IsJSON(r.Header.Get("Content-Type")) {
				WriteError(w, r, E(CodeUnsupportedMediaType))
				return
			}
			if r.ContentLength > MaxJSONBytes {
				WriteError(w, r, E(CodePayloadTooLarge))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, MaxJSONBytes)
		}
		next.ServeHTTP(w, r)
	})
}
