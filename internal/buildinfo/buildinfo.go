// Package buildinfo expõe a versão do binário, gravada no link com
// -ldflags "-X github.com/marquinhosvideo/processosplatdistr/internal/buildinfo.Version=<sha>".
package buildinfo

import "runtime/debug"

// Version identifica o build (sha do commit). Vai no header X-App-Version e
// no /healthz. Sem ldflags, usa a revisão VCS embutida pelo go build ou "dev".
var Version = ""

func init() {
	if Version == "" {
		Version = fromVCS(debug.ReadBuildInfo())
	}
}

func fromVCS(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return "dev"
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
		return "dev"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
