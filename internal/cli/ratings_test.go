package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

func TestRatingsCommand(t *testing.T) {
	svc := service.New(service.Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../testdata/snapshot")) },
		Params: func() (rating.File, error) {
			return rating.File{Params: rating.Params{K: 10, KNew: 24, NewBouts: 45, R: 0.3, WarmupScale: 0.5}, Show: true}, nil
		},
	})
	root := NewRoot(buildinfo.Info{Version: "devel"}, svc)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ratings", "-t", "aki2026", "--top", "3"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 || lines[0] != "Ratings after Aki 2026 (202609)" || !strings.HasPrefix(lines[1], "#") || !strings.HasPrefix(lines[2], "1 ") {
		t.Errorf("output:\n%s", out.String())
	}
	root.SetArgs([]string{"ratings", "-t", "bogus"})
	if err := root.Execute(); err == nil {
		t.Error("a bad tournament was accepted")
	}
}

func TestChange(t *testing.T) {
	for d, want := range map[float64]string{12.4: "▲12", -7.6: "▼8", 0.3: "–", -0.4: "–"} {
		if got := change(d); got != want {
			t.Errorf("change(%v) = %q, want %q", d, got, want)
		}
	}
}
