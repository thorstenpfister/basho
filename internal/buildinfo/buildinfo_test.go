package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	stamped := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.0.0-20261013101500-0123456789ab+dirty"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.time", Value: "2026-10-13T10:15:00Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	tests := []struct {
		name string
		ld   Info
		bi   *debug.BuildInfo
		want Info
	}{
		{
			name: "ldflags win over VCS stamping",
			ld:   Info{Version: "v1.2.3", Commit: "abc1234", Date: "2026-11-01T00:00:00Z"},
			bi:   stamped,
			want: Info{Version: "v1.2.3", Commit: "abc1234", Date: "2026-11-01T00:00:00Z"},
		},
		{
			name: "VCS stamping fills what ldflags leave empty",
			ld:   Info{Version: "v1.2.3"},
			bi:   stamped,
			want: Info{Version: "v1.2.3", Commit: "0123456789ab-dirty", Date: "2026-10-13T10:15:00Z"},
		},
		{
			name: "module version from go install",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}},
			want: Info{Version: "v1.0.0"},
		},
		{
			name: "clean tree, no dirty suffix",
			bi: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "fedcba9876543210"},
				{Key: "vcs.modified", Value: "false"},
			}},
			want: Info{Version: "devel", Commit: "fedcba987654"},
		},
		{
			name: "(devel) module version falls back to devel",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want: Info{Version: "devel"},
		},
		{
			name: "no build info at all",
			want: Info{Version: "devel"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.ld, tt.bi); got != tt.want {
				t.Errorf("resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
