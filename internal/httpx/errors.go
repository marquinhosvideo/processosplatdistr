// Package httpx reúne as convenções HTTP da API (SDD §4.1): JSON de entrada e
// saída, erros problem+json (RFC 9457) renderizados num único ponto,
// paginação, validação e os middlewares da cadeia do servidor.
package httpx

import (
	"fmt"
	"net/http"
)

// Códigos de erro da API (§4.1). O "type" do problem é "urn:pp:error:<code>".
const (
	CodeUnauthenticated      = "unauthenticated"
	CodeSessionExpired       = "session_expired"
	CodeAccessRevoked        = "access_revoked"
	CodeForbidden            = "forbidden"
	CodeCSRFFailed           = "csrf_failed"
	CodeNotFound             = "not_found"
	CodeInvalidTransition    = "invalid_transition"
	CodeStaleVersion         = "stale_version"
	CodePossibleDuplicate    = "possible_duplicate"
	CodeQuarterClosed        = "quarter_closed"
	CodeQuarterNotOpen       = "quarter_not_open"
	CodeAnonymized           = "anonymized"
	CodeArchived             = "archived"
	CodePendingEntries       = "pending_entries"
	CodePendingChanged       = "pending_changed"
	CodeBatchConflict        = "batch_conflict"
	CodePreviewOutdated      = "preview_outdated"
	CodeAlreadyExists        = "already_exists"
	CodePayloadTooLarge      = "payload_too_large"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeValidationFailed     = "validation_failed"
	CodeMalwareDetected      = "malware_detected"
	CodeRateLimited          = "rate_limited"
	CodeUnavailable          = "unavailable"
	CodeInternal             = "internal_error"
)

type codeInfo struct {
	status int
	title  string
	detail string
}

// codes dá status, título e detalhe padrão (pt-BR) de cada código.
var codes = map[string]codeInfo{
	CodeUnauthenticated:      {http.StatusUnauthorized, "Não autenticado", "Entre para continuar."},
	CodeSessionExpired:       {http.StatusUnauthorized, "Sessão expirada", "Sua sessão expirou. Entre novamente."},
	CodeAccessRevoked:        {http.StatusUnauthorized, "Acesso revogado", "Seu acesso à plataforma foi revogado."},
	CodeForbidden:            {http.StatusForbidden, "Sem permissão", "Você não tem permissão para esta ação."},
	CodeCSRFFailed:           {http.StatusForbidden, "Requisição bloqueada", "A requisição não veio da própria aplicação."},
	CodeNotFound:             {http.StatusNotFound, "Não encontrado", "Recurso não encontrado."},
	CodeInvalidTransition:    {http.StatusConflict, "Transição inválida", "A mudança de status não é permitida a partir do status atual."},
	CodeStaleVersion:         {http.StatusConflict, "Versão desatualizada", "O registro foi alterado por outra pessoa. Recarregue e tente de novo."},
	CodePossibleDuplicate:    {http.StatusConflict, "Possível duplicidade", "Já existe um cadastro parecido."},
	CodeQuarterClosed:        {http.StatusConflict, "Quarter fechado", "O quarter está fechado."},
	CodeQuarterNotOpen:       {http.StatusConflict, "Quarter não aberto", "O quarter não está aberto para esta ação."},
	CodeAnonymized:           {http.StatusConflict, "Registro anonimizado", "O registro foi anonimizado e não pode ser alterado."},
	CodeArchived:             {http.StatusConflict, "Registro arquivado", "O registro está arquivado."},
	CodePendingEntries:       {http.StatusConflict, "Há pendências", "Existem registros pendentes."},
	CodePendingChanged:       {http.StatusConflict, "Pendências mudaram", "A lista de pendências mudou. Revise e confirme de novo."},
	CodeBatchConflict:        {http.StatusConflict, "Conflito no lote", "Um ou mais itens do lote não podem ser processados; nada foi alterado."},
	CodePreviewOutdated:      {http.StatusConflict, "Prévia desatualizada", "Os dados mudaram desde a prévia. Gere uma nova prévia."},
	CodeAlreadyExists:        {http.StatusConflict, "Já existe", "O registro já existe."},
	CodePayloadTooLarge:      {http.StatusRequestEntityTooLarge, "Conteúdo grande demais", "O conteúdo enviado excede o tamanho máximo."},
	CodeUnsupportedMediaType: {http.StatusUnsupportedMediaType, "Tipo de conteúdo não suportado", "Envie o corpo como application/json."},
	CodeValidationFailed:     {http.StatusUnprocessableEntity, "Dados inválidos", "Corrija os campos destacados."},
	CodeMalwareDetected:      {http.StatusUnprocessableEntity, "Arquivo recusado", "O arquivo foi recusado pela verificação de segurança."},
	CodeRateLimited:          {http.StatusTooManyRequests, "Muitas requisições", "Muitas tentativas. Aguarde um pouco e tente de novo."},
	CodeUnavailable:          {http.StatusServiceUnavailable, "Serviço indisponível", "Serviço temporariamente indisponível. Tente de novo em instantes."},
	CodeInternal:             {http.StatusInternalServerError, "Erro interno", ""},
}

// Error é um erro de API com código conhecido. Handlers o devolvem e
// WriteError o transforma em problem+json. Cada chamada a E (ou aos atalhos)
// cria uma instância nova; não guarde instâncias em variáveis de pacote, porque
// os métodos With* alteram o receptor.
type Error struct {
	Status int
	Code   string
	Detail string            // pt-BR, para a pessoa usuária
	Errors map[string]string // erros por campo (422)
	Ext    map[string]any    // membros de extensão: matches, id…
	Cause  error             // só vai para o log, nunca para a resposta
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%d %s", e.Status, e.Code)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Cause }

// E cria um erro com o código dado e seu status e detalhe padrão. Um código
// desconhecido vira 500 (bug de programação).
func E(code string) *Error {
	info, ok := codes[code]
	if !ok {
		return &Error{Status: http.StatusInternalServerError, Code: CodeInternal, Cause: fmt.Errorf("código de erro desconhecido %q", code)}
	}
	return &Error{Status: info.status, Code: code, Detail: info.detail}
}

// WithDetail troca o detalhe padrão.
func (e *Error) WithDetail(format string, args ...any) *Error {
	e.Detail = fmt.Sprintf(format, args...)
	return e
}

// With acrescenta um membro de extensão ao problem (ex.: "matches", "id").
func (e *Error) With(key string, value any) *Error {
	if e.Ext == nil {
		e.Ext = map[string]any{}
	}
	e.Ext[key] = value
	return e
}

// WithCause anexa a causa, registrada no log.
func (e *Error) WithCause(err error) *Error {
	e.Cause = err
	return e
}

// Atalhos para os casos mais comuns.

func NotFound() *Error            { return E(CodeNotFound) }
func Forbidden() *Error           { return E(CodeForbidden) }
func Internal(cause error) *Error { return E(CodeInternal).WithCause(cause) }

// Validation cria um 422 com erros por campo.
func Validation(fields map[string]string) *Error {
	e := E(CodeValidationFailed)
	e.Errors = fields
	return e
}
