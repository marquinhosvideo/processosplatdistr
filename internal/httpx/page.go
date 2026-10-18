package httpx

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Limites da paginação (§4.1).
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// Page é a página pedida em ?page=&pageSize=.
type Page struct {
	Number int // a partir de 1
	Size   int
}

// Offset e Limit para o SQL.
func (p Page) Offset() int { return (p.Number - 1) * p.Size }
func (p Page) Limit() int  { return p.Size }

// ParsePage lê page (default 1) e pageSize (default 25, máximo 100). Valores
// fora da faixa dão 422.
func ParsePage(q url.Values) (Page, error) {
	p := Page{Number: 1, Size: DefaultPageSize}
	v := &V{}
	if s := strings.TrimSpace(q.Get("page")); s != "" {
		n, err := strconv.Atoi(s)
		v.Check(err == nil && n >= 1 && n <= 1_000_000, "page", "deve ser um número inteiro a partir de 1")
		p.Number = n
	}
	if s := strings.TrimSpace(q.Get("pageSize")); s != "" {
		n, err := strconv.Atoi(s)
		v.Check(err == nil && n >= 1 && n <= MaxPageSize, "pageSize", fmt.Sprintf("deve estar entre 1 e %d", MaxPageSize))
		p.Size = n
	}
	if err := v.Err(); err != nil {
		return Page{}, err
	}
	return p, nil
}

// PageResult é o envelope das listas paginadas.
type PageResult[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}

// NewPageResult monta o envelope; items nil vira [] no JSON.
func NewPageResult[T any](items []T, p Page, total int64) PageResult[T] {
	if items == nil {
		items = []T{}
	}
	return PageResult[T]{Items: items, Page: p.Number, PageSize: p.Size, Total: total}
}

// ParseSort lê ?sort= contra a allowlist do endpoint. O primeiro valor
// permitido é o default. Os valores já trazem o prefixo "-" quando são
// decrescentes (ex.: "-updatedAt", "name"); cada módulo mapeia o valor
// devolvido para o ORDER BY.
func ParseSort(q url.Values, allowed ...string) (string, error) {
	if len(allowed) == 0 {
		panic("httpx.ParseSort: allowlist vazia")
	}
	s := strings.TrimSpace(q.Get("sort"))
	if s == "" {
		return allowed[0], nil
	}
	if !slices.Contains(allowed, s) {
		return "", Validation(map[string]string{"sort": "ordenação não permitida (use " + strings.Join(allowed, ", ") + ")"})
	}
	return s, nil
}
