package buildinfo

import (
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func settings(kv ...string) *debug.BuildInfo {
	info := &debug.BuildInfo{}
	for i := 0; i < len(kv); i += 2 {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
	}
	return info
}

func TestFromVCS(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"sem build info", nil, false, ""},
		{"sem revisão", settings(), true, ""},
		{"revisão curta", settings("vcs.revision", "abc123"), true, "abc123"},
		{"revisão longa", settings("vcs.revision", "0123456789abcdef0123"), true, "0123456789ab"},
		{"árvore suja", settings("vcs.revision", "0123456789abcdef", "vcs.modified", "true"), true, "0123456789ab-dirty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromVCS(tt.info, tt.ok); got != tt.want {
				t.Errorf("fromVCS = %q, want %q", got, tt.want)
			}
		})
	}
}

// Na imagem do Tsuru não há ldflags nem VCS: a versão sai do conteúdo do
// executável e muda quando ele muda.
func TestResolve(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "server")
	if err := os.WriteFile(bin, []byte("binario v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	noVCS := func() (*debug.BuildInfo, bool) { return settings(), true }
	exe := func() (string, error) { return bin, nil }

	if got := resolve(func() (*debug.BuildInfo, bool) { return settings("vcs.revision", "0123456789abcdef"), true }, exe); got != "0123456789ab" {
		t.Errorf("com VCS: %q", got)
	}
	// sha256("binario v1")
	v1 := resolve(noVCS, exe)
	if v1 != "bin-4fa4fae1ac26" {
		t.Errorf("sem VCS: %q", v1)
	}
	if err := os.WriteFile(bin, []byte("binario v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v2 := resolve(noVCS, exe); v2 == v1 || len(v2) != len(v1) {
		t.Errorf("versão não mudou com o binário: %q -> %q", v1, v2)
	}
	if got := resolve(noVCS, func() (string, error) { return filepath.Join(dir, "sumiu"), nil }); got != "dev" {
		t.Errorf("executável ilegível: %q", got)
	}
	if got := resolve(noVCS, func() (string, error) { return "", errors.New("sem /proc") }); got != "dev" {
		t.Errorf("sem executável: %q", got)
	}
}

func TestVersionNeverEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version vazia depois do init")
	}
}
