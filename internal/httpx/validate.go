package httpx

import (
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// DateLayout é o formato de DATE na API.
const DateLayout = "2006-01-02"

// V acumula erros de formato por campo e vira um 422 validation_failed.
// Só o primeiro erro de cada campo é mantido. As regras de formato opcional
// (Email, HTTPURL, Date, OneOf) ignoram valor vazio; combine com Required
// quando o campo for obrigatório. Invariantes de negócio ficam no model.go
// de cada módulo, não aqui.
//
//	v := &httpx.V{}
//	v.Required("fullName", in.FullName).MaxLen("fullName", in.FullName, 200)
//	v.Email("email", in.Email)
//	if err := v.Err(); err != nil { return err }
type V struct {
	errs map[string]string
}

// Check registra msg para field quando ok é falso.
func (v *V) Check(ok bool, field, msg string) *V {
	if !ok {
		v.add(field, msg)
	}
	return v
}

func (v *V) add(field, msg string) {
	if v.errs == nil {
		v.errs = map[string]string{}
	}
	if _, dup := v.errs[field]; !dup {
		v.errs[field] = msg
	}
}

// Required exige texto não vazio (espaços não contam).
func (v *V) Required(field, value string) *V {
	return v.Check(strings.TrimSpace(value) != "", field, "obrigatório")
}

// MaxLen limita o tamanho em caracteres (runes), não em bytes.
func (v *V) MaxLen(field, value string, n int) *V {
	return v.Check(utf8.RuneCountInString(value) <= n, field, fmt.Sprintf("deve ter no máximo %d caracteres", n))
}

// Email valida um endereço simples, sem nome de exibição.
func (v *V) Email(field, value string) *V {
	if value == "" {
		return v
	}
	return v.Check(validEmail(value), field, "e-mail inválido")
}

func validEmail(s string) bool {
	if len(s) > 254 || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || addr.Name != "" {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	domain := s[at+1:]
	return at > 0 && strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// OneOf exige um dos valores permitidos (enums).
func (v *V) OneOf(field, value string, allowed ...string) *V {
	if value == "" {
		return v
	}
	return v.Check(slices.Contains(allowed, value), field, "valor inválido")
}

// HTTPURL exige URL absoluta http ou https, com host e sem credenciais.
func (v *V) HTTPURL(field, value string) *V {
	if value == "" {
		return v
	}
	u, err := url.Parse(value)
	ok := err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil &&
		!strings.ContainsAny(value, " \t\r\n")
	return v.Check(ok, field, "URL inválida (use http:// ou https://)")
}

// Range exige lo ≤ n ≤ hi.
func (v *V) Range(field string, n, lo, hi int64) *V {
	return v.Check(n >= lo && n <= hi, field, fmt.Sprintf("deve estar entre %d e %d", lo, hi))
}

// Date exige uma data de calendário válida em AAAA-MM-DD.
func (v *V) Date(field, value string) *V {
	if value == "" {
		return v
	}
	t, err := time.Parse(DateLayout, value)
	return v.Check(err == nil && t.Format(DateLayout) == value, field, "data inválida (use AAAA-MM-DD)")
}

// Valid informa se nenhum erro foi registrado.
func (v *V) Valid() bool { return len(v.errs) == 0 }

// Err devolve nil ou um 422 validation_failed com os erros por campo.
func (v *V) Err() error {
	if v.Valid() {
		return nil
	}
	return Validation(v.errs)
}
