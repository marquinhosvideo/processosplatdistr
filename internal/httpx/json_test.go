package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type sample struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Address struct {
		Zip string `json:"zip"`
	} `json:"address"`
}

func decodeBody(contentType, body string) (sample, error) {
	r := httptest.NewRequest("POST", "/api/v1/x", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	var s sample
	err := DecodeJSON(httptest.NewRecorder(), r, &s)
	return s, err
}

func apiErr(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("erro = %v, want *httpx.Error", err)
	}
	return e
}

func TestDecodeJSONOK(t *testing.T) {
	for _, ct := range []string{"application/json", "application/json; charset=utf-8", "Application/JSON"} {
		s, err := decodeBody(ct, ` {"name":"Ana","age":30,"address":{"zip":"01000"}} `+"\n")
		if err != nil {
			t.Fatalf("%s: %v", ct, err)
		}
		if s.Name != "Ana" || s.Age != 30 || s.Address.Zip != "01000" {
			t.Errorf("decodificado = %+v", s)
		}
	}
}

func TestDecodeJSONErrors(t *testing.T) {
	big := `{"name":"` + strings.Repeat("x", MaxJSONBytes) + `"}`
	tests := []struct {
		name, ct, body string
		code           string
		field, msg     string
	}{
		{"sem content-type", "", `{}`, CodeUnsupportedMediaType, "", ""},
		{"form", "application/x-www-form-urlencoded", `name=x`, CodeUnsupportedMediaType, "", ""},
		{"text/plain", "text/plain", `{}`, CodeUnsupportedMediaType, "", ""},
		{"campo desconhecido", "application/json", `{"name":"x","isAdmin":true}`, CodeValidationFailed, "isAdmin", "campo desconhecido"},
		{"tipo errado", "application/json", `{"age":"trinta"}`, CodeValidationFailed, "age", "tipo inválido"},
		{"tipo errado aninhado", "application/json", `{"address":{"zip":1}}`, CodeValidationFailed, "address.zip", "tipo inválido"},
		{"sintaxe", "application/json", `{"name":`, CodeValidationFailed, "", ""},
		{"lixo", "application/json", `isso não é json`, CodeValidationFailed, "", ""},
		{"vazio", "application/json", ``, CodeValidationFailed, "", ""},
		{"dois objetos", "application/json", `{} {}`, CodeValidationFailed, "", ""},
		{"lixo depois", "application/json", `{}]`, CodeValidationFailed, "", ""},
		{"array", "application/json", `[1,2]`, CodeValidationFailed, "", ""},
		{"grande demais", "application/json", big, CodePayloadTooLarge, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeBody(tt.ct, tt.body)
			e := apiErr(t, err)
			if e.Code != tt.code {
				t.Fatalf("code = %s, want %s (%v)", e.Code, tt.code, err)
			}
			if tt.field != "" && e.Errors[tt.field] != tt.msg {
				t.Errorf("errors = %v, want %s=%q", e.Errors, tt.field, tt.msg)
			}
			if e.Status == 422 && e.Detail == "" {
				t.Error("422 sem detail")
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, httptest.NewRequest("GET", "/", nil), http.StatusCreated, map[string]any{"id": 7, "html": "<b>"})
	if rec.Code != 201 || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("status=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["id"] != float64(7) {
		t.Errorf("corpo = %s (%v)", rec.Body, err)
	}
	if strings.Contains(rec.Body.String(), "<b>") {
		t.Error("HTML não escapado no JSON")
	}
	if rec.Header().Get("Content-Length") != "" && rec.Header().Get("Content-Length") != strconv.Itoa(rec.Body.Len()) {
		t.Errorf("Content-Length = %s, corpo = %d", rec.Header().Get("Content-Length"), rec.Body.Len())
	}

	rec = httptest.NewRecorder()
	WriteJSON(rec, httptest.NewRequest("HEAD", "/", nil), http.StatusOK, map[string]string{"a": "b"})
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD com corpo: %q", rec.Body)
	}

	var logs bytes.Buffer
	rec = httptest.NewRecorder()
	WriteJSON(rec, newReq("GET", "/", &logs), http.StatusOK, map[string]any{"f": func() {}})
	if rec.Code != 500 {
		t.Errorf("valor não serializável: status = %d", rec.Code)
	}
}

func TestRequireJSON(t *testing.T) {
	var reached bool
	h := RequireJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		var s sample
		if err := DecodeJSON(w, r, &s); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	tests := []struct {
		method, ct string
		body       string
		length     int64 // -1 = do corpo
		want       int
		reached    bool
	}{
		{"GET", "", "", -1, 415, true}, // leitura passa; aqui é o DecodeJSON que recusa
		{"POST", "application/json", `{"name":"x"}`, -1, 204, true},
		{"POST", "application/json; charset=utf-8", `{"name":"x"}`, -1, 204, true},
		{"POST", "", `{"name":"x"}`, -1, 415, false},
		{"POST", "text/plain", `{"name":"x"}`, -1, 415, false},
		{"POST", "multipart/form-data; boundary=x", `--x--`, -1, 415, false},
		{"PUT", "application/x-www-form-urlencoded", `a=b`, -1, 415, false},
		{"DELETE", "", "", -1, 415, false},
		{"PATCH", "application/json", `{}`, -1, 204, true},
		{"POST", "application/json", `{}`, MaxJSONBytes + 1, 413, false},
	}
	for _, tt := range tests {
		reached = false
		r := httptest.NewRequest(tt.method, "/api/v1/x", strings.NewReader(tt.body))
		if tt.ct != "" {
			r.Header.Set("Content-Type", tt.ct)
		}
		if tt.length >= 0 {
			r.ContentLength = tt.length
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != tt.want || reached != tt.reached {
			t.Errorf("%s %q: status=%d reached=%v, want %d %v (%s)", tt.method, tt.ct, rec.Code, reached, tt.want, tt.reached, rec.Body)
		}
	}
}

func TestRequireJSONLimitsChunkedBody(t *testing.T) {
	h := RequireJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var s sample
		if err := DecodeJSON(w, r, &s); err != nil {
			WriteError(w, r, err)
		}
	}))
	body := `{"name":"` + strings.Repeat("x", MaxJSONBytes) + `"}`
	r := httptest.NewRequest("POST", "/api/v1/x", strings.NewReader(body))
	r.ContentLength = -1 // tamanho desconhecido (chunked)
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 413 {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}
