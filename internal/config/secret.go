package config

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
)

const redacted = "[REDACTED]"

// Secret guarda um valor sensível (senha, chave, token, DSN). Qualquer forma
// de impressão — fmt, slog, JSON, texto — mostra "[REDACTED]" (ou "" quando
// vazio). O valor real só sai por Reveal, que deve ser usado apenas no ponto
// de consumo (driver, cliente OIDC, seal).
type Secret string

// Reveal devolve o valor em claro.
func (s Secret) Reveal() string { return string(s) }

// IsSet informa se o segredo foi definido.
func (s Secret) IsSet() bool { return s != "" }

func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

// GoString cobre %#v.
func (s Secret) GoString() string { return "config.Secret(" + strconv.Quote(s.String()) + ")" }

// Format garante a redação em qualquer verbo do fmt (%s, %v, %q, %x, %d…).
func (s Secret) Format(f fmt.State, verb rune) {
	switch {
	case verb == 'v' && f.Flag('#'):
		_, _ = io.WriteString(f, s.GoString())
	case verb == 'q':
		_, _ = io.WriteString(f, strconv.Quote(s.String()))
	default:
		_, _ = io.WriteString(f, s.String())
	}
}

// LogValue implementa slog.LogValuer.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }
