package httpx

import (
	"strings"
	"testing"
)

func TestValidatorRules(t *testing.T) {
	tests := []struct {
		name string
		run  func(v *V)
		want string // "" = válido
	}{
		{"required ok", func(v *V) { v.Required("f", "x") }, ""},
		{"required vazio", func(v *V) { v.Required("f", "") }, "obrigatório"},
		{"required só espaços", func(v *V) { v.Required("f", " \t ") }, "obrigatório"},

		{"maxlen no limite em runes", func(v *V) { v.MaxLen("f", "ação", 4) }, ""},
		{"maxlen acima", func(v *V) { v.MaxLen("f", "ações", 4) }, "deve ter no máximo 4 caracteres"},

		{"email ok", func(v *V) { v.Email("f", "ana.souza+rh@exemplo.com.br") }, ""},
		{"email vazio é opcional", func(v *V) { v.Email("f", "") }, ""},
		{"email sem arroba", func(v *V) { v.Email("f", "ana.exemplo.com") }, "e-mail inválido"},
		{"email sem ponto no domínio", func(v *V) { v.Email("f", "ana@localhost") }, "e-mail inválido"},
		{"email com nome", func(v *V) { v.Email("f", "Ana <ana@exemplo.com>") }, "e-mail inválido"},
		{"email com espaço", func(v *V) { v.Email("f", "ana @exemplo.com") }, "e-mail inválido"},
		{"email com CRLF", func(v *V) { v.Email("f", "ana@exemplo.com\r\nBcc: x@y.z") }, "e-mail inválido"},
		{"email domínio terminando em ponto", func(v *V) { v.Email("f", "ana@exemplo.") }, "e-mail inválido"},
		{"email longo", func(v *V) { v.Email("f", strings.Repeat("a", 250)+"@x.com") }, "e-mail inválido"},

		{"oneof ok", func(v *V) { v.OneOf("f", "sr", "jr", "pl", "sr") }, ""},
		{"oneof vazio é opcional", func(v *V) { v.OneOf("f", "", "jr") }, ""},
		{"oneof fora", func(v *V) { v.OneOf("f", "senior", "jr", "pl", "sr") }, "valor inválido"},

		{"url https", func(v *V) { v.HTTPURL("f", "https://www.linkedin.com/in/ana") }, ""},
		{"url http", func(v *V) { v.HTTPURL("f", "http://exemplo.com") }, ""},
		{"url javascript", func(v *V) { v.HTTPURL("f", "javascript:alert(1)") }, "URL inválida (use http:// ou https://)"},
		{"url relativa", func(v *V) { v.HTTPURL("f", "/in/ana") }, "URL inválida (use http:// ou https://)"},
		{"url sem host", func(v *V) { v.HTTPURL("f", "https://") }, "URL inválida (use http:// ou https://)"},
		{"url com credencial", func(v *V) { v.HTTPURL("f", "https://u:p@exemplo.com") }, "URL inválida (use http:// ou https://)"},
		{"url com espaço", func(v *V) { v.HTTPURL("f", "https://exemplo.com/a b") }, "URL inválida (use http:// ou https://)"},

		{"range ok", func(v *V) { v.Range("f", 5, 1, 5) }, ""},
		{"range abaixo", func(v *V) { v.Range("f", 0, 1, 5) }, "deve estar entre 1 e 5"},
		{"range acima", func(v *V) { v.Range("f", 6, 1, 5) }, "deve estar entre 1 e 5"},

		{"data ok", func(v *V) { v.Date("f", "2026-02-28") }, ""},
		{"data bissexta", func(v *V) { v.Date("f", "2028-02-29") }, ""},
		{"data inexistente", func(v *V) { v.Date("f", "2026-02-30") }, "data inválida (use AAAA-MM-DD)"},
		{"data em pt-BR", func(v *V) { v.Date("f", "28/02/2026") }, "data inválida (use AAAA-MM-DD)"},
		{"data sem zero", func(v *V) { v.Date("f", "2026-2-1") }, "data inválida (use AAAA-MM-DD)"},
		{"data com hora", func(v *V) { v.Date("f", "2026-02-01T00:00:00Z") }, "data inválida (use AAAA-MM-DD)"},

		{"check", func(v *V) { v.Check(false, "f", "regra própria") }, "regra própria"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &V{}
			tt.run(v)
			err := v.Err()
			if tt.want == "" {
				if err != nil || !v.Valid() {
					t.Fatalf("esperava válido, veio %v", err)
				}
				return
			}
			e := apiErr(t, err)
			if e.Status != 422 || e.Code != CodeValidationFailed || e.Errors["f"] != tt.want {
				t.Errorf("erro = %d %s %v, want f=%q", e.Status, e.Code, e.Errors, tt.want)
			}
		})
	}
}

func TestValidatorKeepsFirstErrorPerField(t *testing.T) {
	v := &V{}
	v.Required("nome", "").MaxLen("nome", "", 10)
	v.Required("email", "x").Email("email", "x")
	v.Range("idade", 200, 0, 120)
	e := apiErr(t, v.Err())
	want := map[string]string{"nome": "obrigatório", "email": "e-mail inválido", "idade": "deve estar entre 0 e 120"}
	if len(e.Errors) != len(want) {
		t.Fatalf("errors = %v", e.Errors)
	}
	for k, msg := range want {
		if e.Errors[k] != msg {
			t.Errorf("%s = %q, want %q", k, e.Errors[k], msg)
		}
	}
	// Required falhou primeiro; o MaxLen seguinte não sobrescreve.
	v.Check(false, "nome", "outra")
	if e := apiErr(t, v.Err()); e.Errors["nome"] != "obrigatório" {
		t.Errorf("primeiro erro sobrescrito: %q", e.Errors["nome"])
	}
}
