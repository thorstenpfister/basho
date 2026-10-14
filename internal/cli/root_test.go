package cli

import (
	"bytes"
	"testing"

	"github.com/thorstenpfister/basho/internal/buildinfo"
)

func TestUnknownArgumentFails(t *testing.T) {
	root := NewRoot(buildinfo.Info{Version: "devel"}, nil)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"bogus"})
	if err := root.Execute(); err == nil {
		t.Fatal("basho bogus succeeded, want an error")
	}
}

func TestVersionOutput(t *testing.T) {
	tests := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{
			name: "full",
			info: buildinfo.Info{Version: "v1.2.3", Commit: "0123456789ab-dirty", Date: "2026-10-13T10:15:00Z"},
			want: "basho v1.2.3\n  commit  0123456789ab-dirty\n  date    2026-10-13T10:15:00Z\n",
		},
		{
			name: "devel without VCS data",
			info: buildinfo.Info{Version: "devel"},
			want: "basho devel\n",
		},
	}
	for _, tt := range tests {
		// The subcommand and the flag must print the same text.
		for _, args := range [][]string{{"version"}, {"--version"}} {
			t.Run(tt.name+" "+args[0], func(t *testing.T) {
				root := NewRoot(tt.info, nil)
				var out bytes.Buffer
				root.SetOut(&out)
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("execute: %v", err)
				}
				if out.String() != tt.want {
					t.Errorf("output =\n%q\nwant\n%q", out.String(), tt.want)
				}
			})
		}
	}
}
