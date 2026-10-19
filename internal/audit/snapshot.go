package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Valores da tag audit.
const (
	TagPlain    = "plain"
	TagRedacted = "redacted"
)

// changed substitui o valor de um campo redacted.
var changed = json.RawMessage(`{"changed":true}`)

type snapField struct {
	name  string
	plain bool
	value json.RawMessage
	zero  bool
}

// Diff calcula before_json e after_json (nil = NULL) a partir dos snapshots.
//   - Atualização (os dois presentes): só os campos que mudaram; plain com o
//     valor antigo e o novo, redacted como {"changed":true} nos dois lados.
//   - Criação (before nil): todos os plain em after, e os redacted
//     preenchidos como {"changed":true}.
//   - Exclusão (after nil): o simétrico, em before.
func Diff(before, after any) (beforeJSON, afterJSON []byte, err error) {
	bf, err := snapshotFields(before)
	if err != nil {
		return nil, nil, err
	}
	af, err := snapshotFields(after)
	if err != nil {
		return nil, nil, err
	}
	b, a := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	switch {
	case bf == nil && af == nil:
	case bf == nil:
		fill(a, af)
	case af == nil:
		fill(b, bf)
	default:
		old := map[string]snapField{}
		for _, f := range bf {
			old[f.name] = f
		}
		for _, f := range af {
			o, ok := old[f.name]
			delete(old, f.name)
			if ok && bytes.Equal(o.value, f.value) {
				continue
			}
			plain := f.plain && (!ok || o.plain)
			switch {
			case !plain:
				b[f.name], a[f.name] = changed, changed
			case ok:
				b[f.name], a[f.name] = o.value, f.value
			default:
				a[f.name] = f.value
			}
		}
		for name, o := range old { // campos que só existem no antes
			if o.plain {
				b[name] = o.value
			} else {
				b[name] = changed
			}
		}
	}
	if beforeJSON, err = marshalNonEmpty(b); err != nil {
		return nil, nil, err
	}
	if afterJSON, err = marshalNonEmpty(a); err != nil {
		return nil, nil, err
	}
	return beforeJSON, afterJSON, nil
}

func fill(dst map[string]json.RawMessage, fs []snapField) {
	for _, f := range fs {
		switch {
		case f.plain:
			dst[f.name] = f.value
		case !f.zero:
			dst[f.name] = changed
		}
	}
}

func marshalNonEmpty(m map[string]json.RawMessage) ([]byte, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return json.Marshal(m)
}

// snapshotFields lê os campos exportados de um snapshot (struct ou ponteiro
// para struct). nil, ou ponteiro nil, devolve nil.
func snapshotFields(v any) ([]snapField, error) {
	if v == nil {
		return nil, nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: snapshot deve ser struct, recebeu %T", ErrInvalid, v)
	}
	rt := rv.Type()
	warnUntagged(rt)
	out := make([]snapField, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		name := fieldName(sf)
		if name == "" {
			continue
		}
		fv := rv.Field(i)
		raw, err := json.Marshal(fv.Interface())
		if err != nil {
			return nil, fmt.Errorf("%w: campo %s.%s: %v", ErrInvalid, rt.Name(), sf.Name, err)
		}
		out = append(out, snapField{name: name, plain: sf.Tag.Get("audit") == TagPlain, value: raw, zero: fv.IsZero()})
	}
	return out, nil
}

// fieldName usa o nome da tag json; sem ela, o nome Go com a inicial
// minúscula (camelCase da API). json:"-" exclui o campo.
func fieldName(sf reflect.StructField) string {
	if tag, ok := sf.Tag.Lookup("json"); ok {
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			return ""
		}
		if name != "" {
			return name
		}
	}
	r, size := utf8.DecodeRuneInString(sf.Name)
	return string(unicode.ToLower(r)) + sf.Name[size:]
}

// CheckSnapshot lista os campos exportados de t sem tag audit válida. Os
// módulos podem usá-lo nos próprios testes; o teste do pacote audit varre o
// código-fonte inteiro.
func CheckSnapshot(t reflect.Type) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return []string{t.String() + ": snapshot deve ser struct"}
	}
	var bad []string
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		if tag := sf.Tag.Get("audit"); tag != TagPlain && tag != TagRedacted {
			bad = append(bad, fmt.Sprintf("%s.%s: tag audit:\"plain\" ou audit:\"redacted\" ausente ou inválida (%q)", t.Name(), sf.Name, tag))
		}
	}
	return bad
}

var warned sync.Map // reflect.Type → struct{}

// warnUntagged avisa uma vez por tipo quando um snapshot tem campo sem tag;
// o campo é gravado como redacted.
func warnUntagged(t reflect.Type) {
	if _, done := warned.Load(t); done {
		return
	}
	warned.Store(t, struct{}{})
	for _, problem := range CheckSnapshot(t) {
		slog.Warn("audit.untagged_field", slog.String("detail", problem+"; gravado como redacted"))
	}
}
