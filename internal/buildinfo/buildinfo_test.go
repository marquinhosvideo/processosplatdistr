package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFromVCS(t *testing.T) {
	settings := func(kv ...string) *debug.BuildInfo {
		info := &debug.BuildInfo{}
		for i := 0; i < len(kv); i += 2 {
			info.Settings = append(info.Settings, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return info
	}
	tests := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"sem build info", nil, false, "dev"},
		{"sem revisão", settings(), true, "dev"},
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

func TestVersionNeverEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version vazia depois do init")
	}
}
