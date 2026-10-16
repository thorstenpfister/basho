package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
)

func TestUnknownArgumentFails(t *testing.T) {
	root := NewRoot(buildinfo.Info{Version: "devel"}, Deps{})
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
				root := NewRoot(tt.info, Deps{})
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

// SPEC §14: `basho version` names the snapshot's date and range.
func TestVersionData(t *testing.T) {
	bundle := func() (service.Bundle, error) {
		return service.Bundle{
			Generated: time.Date(2026, 10, 14, 6, 36, 0, 0, time.UTC),
			Oldest:    domain.BashoID{Year: 1960, Month: time.January},
			Newest:    domain.BashoID{Year: 2026, Month: time.September},
		}, nil
	}
	for _, args := range [][]string{{"version"}, {"--version"}} {
		root := NewRoot(buildinfo.Info{Version: "v1.0.0"}, Deps{Bundle: bundle})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if want := "basho v1.0.0\n  data    2026-10-14 (196001–202609)\n"; out.String() != want {
			t.Errorf("%s =\n%q\nwant\n%q", args[0], out.String(), want)
		}
	}
}
