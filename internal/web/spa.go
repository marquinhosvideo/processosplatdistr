package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/marquinhosvideo/processosplatdistr/internal/httpx"
)

// Cache-Control dos arquivos (§7.8). Os assets do Vite têm hash no nome e
// nunca mudam; index.html e os demais arquivos da raiz revalidam pelo ETag.
const (
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"
)

// Tabela própria de tipos: a imagem Alpine não tem /etc/mime.types e, com
// nosniff, um tipo errado impede o navegador de executar o script.
var contentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json",
	".map":         "application/json",
	".webmanifest": "application/manifest+json",
	".txt":         "text/plain; charset=utf-8",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".wasm":        "application/wasm",
}

type asset struct {
	data  []byte
	etag  string
	ctype string
	cache string
}

// SPA serve o build do frontend: arquivos existentes como estão e qualquer
// outro caminho de página com index.html (o roteamento é do React Router).
type SPA struct {
	files map[string]*asset // caminho relativo, sem barra inicial
	index *asset
}

// New carrega os arquivos de fsys em memória e calcula os ETags. fsys deve ter
// index.html na raiz; sem ele, as páginas respondem 503.
func New(fsys fs.FS) (*SPA, error) {
	s := &SPA{files: map[string]*asset{}}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		s.files[name] = &asset{
			data:  data,
			etag:  `"` + base64.RawURLEncoding.EncodeToString(sum[:12]) + `"`,
			ctype: contentType(name),
			cache: cacheFor(name),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("carregar o SPA: %w", err)
	}
	s.index = s.files["index.html"]
	return s, nil
}

// Built informa se o frontend foi compilado (há index.html).
func (s *SPA) Built() bool { return s.index != nil }

func (s *SPA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if reserved(r.URL.Path) {
		// /api, /auth e /s não são páginas do SPA: rota desconhecida é 404 JSON.
		httpx.WriteError(w, r, httpx.NotFound())
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpx.WriteError(w, r, httpx.E(httpx.CodeMethodNotAllowed))
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if a, ok := s.files[name]; ok {
		serve(w, r, a)
		return
	}
	if looksLikeFile(name) {
		// Asset ou arquivo da raiz inexistente não vira index.html: um chunk JS
		// antigo receberia HTML e quebraria com erro confuso no navegador.
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Arquivo não encontrado.", http.StatusNotFound)
		return
	}
	if s.index == nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Frontend não compilado. Rode \"make build\" para gerar o SPA.", http.StatusServiceUnavailable)
		return
	}
	serve(w, r, s.index)
}

func serve(w http.ResponseWriter, r *http.Request, a *asset) {
	h := w.Header()
	h.Set("Content-Type", a.ctype)
	h.Set("Cache-Control", a.cache)
	h.Set("ETag", a.etag)
	// ServeContent trata HEAD, Range e If-None-Match (304).
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(a.data))
}

func reserved(p string) bool {
	for _, prefix := range []string{"/api", "/auth", "/s"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// looksLikeFile vale para /assets/* (onde o Vite põe tudo o que gera) e para
// nomes com extensão na raiz (favicon.ico, robots.txt). Um caminho mais fundo
// com ponto, como /pessoas/ana.souza, continua sendo página do SPA.
func looksLikeFile(name string) bool {
	if name == "assets" || strings.HasPrefix(name, "assets/") {
		return true
	}
	return !strings.Contains(name, "/") && path.Ext(name) != ""
}

func contentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func cacheFor(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return cacheImmutable
	}
	return cacheRevalidate
}
