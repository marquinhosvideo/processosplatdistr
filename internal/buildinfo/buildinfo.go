// Package buildinfo expõe a versão do binário, gravada no link com
// -ldflags "-X github.com/marquinhosvideo/processosplatdistr/internal/buildinfo.Version=<sha>".
package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime/debug"
)

// Version identifica o build. Vai no header X-App-Version e no /healthz, e o
// SPA oferece recarregar quando ela muda. Sem ldflags, usa a revisão VCS
// embutida pelo go build; sem ela (imagem do Tsuru: o deploy não passa
// --build-arg e o .git fica fora do contexto), o hash do próprio executável,
// que muda a cada build com conteúdo diferente. "dev" só se nada disso der.
var Version = ""

func init() {
	if Version == "" {
		Version = resolve(debug.ReadBuildInfo, os.Executable)
	}
}

func resolve(readBuildInfo func() (*debug.BuildInfo, bool), executable func() (string, error)) string {
	if v := fromVCS(readBuildInfo()); v != "" {
		return v
	}
	if path, err := executable(); err == nil {
		if v := fromFile(path); v != "" {
			return v
		}
	}
	return "dev"
}

// fromVCS devolve a revisão (12 caracteres, "-dirty" se a árvore tinha
// mudanças) ou "" sem informação de VCS.
func fromVCS(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return ""
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

// fromFile devolve "bin-" + os 12 primeiros hex do sha256 do arquivo, ou ""
// se não conseguir lê-lo. O build é reprodutível (-trimpath), então o mesmo
// código gera a mesma versão.
func fromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return "bin-" + hex.EncodeToString(h.Sum(nil))[:12]
}
