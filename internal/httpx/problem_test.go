package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newReq cria uma requisição com request ID e um logger que escreve em buf.
func newReq(method, target string, buf *bytes.Buffer) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return r.WithContext(withRequest(r.Context(), "0123456789abcdef", logger))
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Fatalf("Content-Type = %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON: %v (%s)", err, rec.Body)
	}
	return body
}

func TestWriteErrorKnownCode(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	WriteError(rec, newReq("GET", "/api/v1/x", &logs), NotFound())

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	body := decodeProblem(t, rec)
	want := map[string]any{
		"type":      "urn:pp:error:not_found",
		"title":     "Não encontrado",
		"status":    float64(404),
		"code":      "not_found",
		"detail":    "Recurso não encontrado.",
		"requestId": "0123456789abcdef",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, want %v", k, body[k], v)
		}
	}
	if len(body) != len(want) {
		t.Errorf("membros extras: %v", body)
	}
}

func TestWriteErrorValidationAndExtensions(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	err := Validation(map[string]string{"email": "inválido"}).
		With("matches", []int{1, 2}).
		With("status", 200) // membro reservado: ignorado
	WriteError(rec, newReq("POST", "/api/v1/x", &logs), err)

	body := decodeProblem(t, rec)
	if rec.Code != 422 || body["status"] != float64(422) || body["code"] != "validation_failed" {
		t.Fatalf("status=%d body=%v", rec.Code, body)
	}
	if errs, _ := body["errors"].(map[string]any); errs["email"] != "inválido" {
		t.Errorf("errors = %v", body["errors"])
	}
	if m, _ := body["matches"].([]any); len(m) != 2 {
		t.Errorf("matches = %v", body["matches"])
	}
}

func TestWriteErrorCustomDetailAndCauseNotExposed(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	WriteError(rec, newReq("PUT", "/x", &logs), E(CodeStaleVersion).WithDetail("Versão %d.", 3).WithCause(errors.New("linha 42 sumiu")))
	body := decodeProblem(t, rec)
	if rec.Code != 409 || body["detail"] != "Versão 3." {
		t.Errorf("status=%d detail=%v", rec.Code, body["detail"])
	}
	if strings.Contains(rec.Body.String(), "linha 42") {
		t.Error("causa vazou na resposta")
	}
	if !strings.Contains(logs.String(), "linha 42") {
		t.Errorf("causa de 4xx deveria ir para o log em debug: %s", logs.String())
	}
}

func TestWriteErrorInternalHidesEverything(t *testing.T) {
	cases := map[string]error{
		"erro comum":           errors.New("dial tcp: senha=xyz"),
		"Internal":             Internal(errors.New("dial tcp: senha=xyz")),
		"código desconhecido":  E("nao_existe").WithCause(errors.New("dial tcp: senha=xyz")),
		"status inconsistente": &Error{Status: 404, Code: CodeValidationFailed, Detail: "senha=xyz"},
		"500 com extensão":     E(CodeInternal).With("dica", "senha=xyz").WithDetail("senha=xyz"),
		"erro embrulhado":      fmt.Errorf("handler: %w", errors.New("senha=xyz")),
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			rec := httptest.NewRecorder()
			WriteError(rec, newReq("GET", "/api/v1/x", &logs), err)
			body := decodeProblem(t, rec)
			if rec.Code != 500 || body["code"] != CodeInternal {
				t.Fatalf("status=%d body=%v", rec.Code, body)
			}
			if _, ok := body["detail"]; ok {
				t.Errorf("500 com detail: %v", body)
			}
			if strings.Contains(rec.Body.String(), "senha") {
				t.Errorf("500 vazou informação: %s", rec.Body)
			}
			if body["requestId"] != "0123456789abcdef" {
				t.Errorf("requestId = %v", body["requestId"])
			}
			if !strings.Contains(logs.String(), `"msg":"http.error"`) || !strings.Contains(logs.String(), `"request_id":"0123456789abcdef"`) {
				t.Errorf("log do 500 incompleto: %s", logs.String())
			}
		})
	}
}

func TestWriteErrorUnavailableShowsDetailNotCause(t *testing.T) {
	var buf bytes.Buffer
	rec := httptest.NewRecorder()
	WriteError(rec, newReq("GET", "/readyz", &buf), E(CodeUnavailable).WithDetail("Banco de dados indisponível.").
		With("schemaVersion", 3).WithCause(errors.New("dial tcp 10.0.0.5:3306: segredo-interno")))
	body := decodeProblem(t, rec)
	if rec.Code != 503 || body["detail"] != "Banco de dados indisponível." || body["schemaVersion"] != float64(3) {
		t.Errorf("503 = %d %v", rec.Code, body)
	}
	if strings.Contains(rec.Body.String(), "segredo-interno") || !strings.Contains(buf.String(), "segredo-interno") {
		t.Errorf("causa deve ir só para o log: corpo %s", rec.Body)
	}
}

func TestWriteErrorMaxBytes(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	WriteError(rec, newReq("POST", "/x", &logs), fmt.Errorf("ler: %w", &http.MaxBytesError{Limit: 10}))
	if body := decodeProblem(t, rec); rec.Code != 413 || body["code"] != CodePayloadTooLarge {
		t.Errorf("status=%d body=%v", rec.Code, body)
	}
}

func TestWriteErrorHeadHasNoBody(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	WriteError(rec, newReq("HEAD", "/x", &logs), NotFound())
	if rec.Code != 404 || rec.Body.Len() != 0 {
		t.Errorf("status=%d body=%q", rec.Code, rec.Body)
	}
}

func TestWriteErrorClearsDownloadHeaders(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Disposition", "attachment")
	rec.Header().Set("ETag", `"x"`)
	WriteError(rec, newReq("GET", "/x", &logs), Forbidden())
	if rec.Header().Get("Content-Disposition") != "" || rec.Header().Get("ETag") != "" {
		t.Errorf("headers do download sobraram: %v", rec.Header())
	}
}

func TestCodeTableComplete(t *testing.T) {
	for code, info := range codes {
		if info.status < 400 || info.title == "" {
			t.Errorf("%s: status=%d title=%q", code, info.status, info.title)
		}
		if info.detail == "" && info.status < 500 {
			t.Errorf("%s: 4xx sem detalhe padrão", code)
		}
		if e := E(code); e.Code != code || e.Status != info.status {
			t.Errorf("E(%q) = %+v", code, e)
		}
	}
}

func TestErrorUnwrapAndMessage(t *testing.T) {
	cause := errors.New("raiz")
	e := E(CodeAlreadyExists).WithCause(cause)
	if !errors.Is(e, cause) {
		t.Error("Unwrap não expõe a causa")
	}
	if !strings.Contains(e.Error(), "409 already_exists: raiz") {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestHandlerFunc(t *testing.T) {
	var logs bytes.Buffer
	ok := HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	rec := httptest.NewRecorder()
	ok.ServeHTTP(rec, newReq("GET", "/", &logs))
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d", rec.Code)
	}

	fail := HandlerFunc(func(w http.ResponseWriter, r *http.Request) error { return Forbidden() })
	rec = httptest.NewRecorder()
	fail.ServeHTTP(rec, newReq("GET", "/", &logs))
	if body := decodeProblem(t, rec); rec.Code != 403 || body["code"] != CodeForbidden {
		t.Errorf("status=%d body=%v", rec.Code, body)
	}
}
