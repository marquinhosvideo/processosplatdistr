// Package web serve o SPA embutido no binário (SDD §10.1). O Vite gera o build
// em internal/web/dist; só dist/robots.txt é versionado, para o go build
// funcionar num clone limpo.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Dist devolve o build do SPA embutido, com index.html na raiz.
func Dist() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err) // "dist" é um caminho fixo e válido
	}
	return sub
}
