// Package migrations embute os arquivos NNNN_nome.sql do schema (SDD §3.2).
// Um arquivo já mergeado nunca é editado: o runner confere o checksum de cada
// versão aplicada. ALTER, DROP e migração de dados vão sozinhos em arquivo
// próprio, sempre compatíveis com a versão anterior do app.
package migrations

import "embed"

// FS contém só os .sql deste diretório.
//
//go:embed *.sql
var FS embed.FS
