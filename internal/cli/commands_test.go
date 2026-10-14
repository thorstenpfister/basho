package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

var testNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)

// fixtureDeps run the CLI over the fixture snapshot (196001, 202607, 202609)
// with a fixed clock and no config file.
func fixtureDeps(t *testing.T) Deps {
	t.Helper()
	cfgHome := t.TempDir()
	return Deps{
		Open: func(cfg config.Config) (*service.Service, error) {
			return service.New(service.Options{
				Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../testdata/snapshot")) },
				Params: func() (rating.File, error) {
					return rating.File{Schema: rating.FileSchema, Params: rating.Params{K: 10, KNew: 24, NewBouts: 45, R: 0.3, WarmupScale: 0.5}, Show: true}, nil
				},
				Clock: clock.NewFake(testNow),
			}), nil
		},
		Env: func(k string) (string, bool) {
			if k == "XDG_CONFIG_HOME" {
				return cfgHome, true
			}
			return "", false
		},
		Now: func() time.Time { return testNow },
	}
}

// run executes basho with args and returns stdout and the exit code.
func run(t *testing.T, deps Deps, args ...string) (string, int) {
	t.Helper()
	root := NewRoot(buildinfo.Info{Version: "devel"}, deps)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	code := exitCode(root.Execute())
	return out.String(), code
}

func TestCommands(t *testing.T) {
	deps := fixtureDeps(t)
	for _, tt := range []struct {
		args []string
		want []string
	}{
		{[]string{"day", "13", "-t", "202609"}, []string{"Aki 2026 · Makuuchi · Day 13 · Fri 2026-09-25", "Onosato", "結び", "Kyujo:", "source: snapshot"}},
		{[]string{"day", "15", "-t", "202609", "--division", "juryo"}, []string{"Juryo · Day 15"}},
		{[]string{"bout", "13", "19", "-t", "aki2026"}, []string{"MUSUBI-NO-ICHIBAN", "ONOSATO", "大の里", "WIN CHANCE", "KIRISHIMA wins by SHITATENAGE", "MOVESET"}},
		{[]string{"standings", "--day", "15", "-t", "2026-09"}, []string{"Standings after Day 15", "Leaders", "Yusho: Onosato"}},
		{[]string{"standings", "--day", "15", "-t", "2026-09", "--grid"}, []string{"W-L-A", "0-0-15", "休"}},
		{[]string{"standings", "-t", "202609"}, []string{"after Day 15"}}, // the default day of a finished tournament
		{[]string{"info", "202609"}, []string{"Aki 2026 (202609)", "Makuuchi  Onosato (Y1e)", "Kanto-sho"}},
		{[]string{"list", "--from", "2026"}, []string{"202609", "202607", "Aonishiki"}},
		{[]string{"ratings", "-t", "202609", "--top", "3"}, []string{"Ratings after Aki 2026", "not betting advice"}},
		{[]string{"credits"}, []string{"sumo-api.com", "osada.us"}},
		{[]string{"day", "13", "-t", "202609", "--ascii"}, []string{"musubi", "Kyujo:"}},
	} {
		out, code := run(t, deps, tt.args...)
		if code != exitOK {
			t.Errorf("%v: exit %d\n%s", tt.args, code, out)
			continue
		}
		for _, w := range tt.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v: output lacks %q:\n%s", tt.args, w, out)
			}
		}
		if strings.Contains(out, "\x1b[") {
			t.Errorf("%v: colour codes in piped output", tt.args)
		}
	}
	if out, _ := run(t, deps, "day", "13", "-t", "202609", "--ascii"); strings.ContainsAny(out, "○●結") {
		t.Errorf("--ascii left symbols:\n%s", out)
	}
}

func TestUsageErrors(t *testing.T) {
	deps := fixtureDeps(t)
	for _, args := range [][]string{
		{"day", "13", "-t", "bogus"},
		{"day", "x", "-t", "202609"},
		{"day", "13", "--division", "makushita"},
		{"bout", "13"},
		{"ratings", "--source", "jsa"},
	} {
		if _, code := run(t, deps, args...); code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
	}
	bad := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(bad, []byte("[ui]\ncolour = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := run(t, deps, "credits"); code != exitOK {
		t.Error("credits needs no config")
	}
	if _, code := run(t, deps, "list", "--config", bad); code != exitUsage {
		t.Errorf("an unknown setting: exit %d", code)
	}
	if _, code := run(t, deps, "list", "--config", filepath.Join(t.TempDir(), "missing.toml")); code != exitUsage {
		t.Errorf("a missing --config: exit %d", code)
	}
}

func TestConfigReachesTheService(t *testing.T) {
	deps := fixtureDeps(t)
	var got config.Config
	open := deps.Open
	deps.Open = func(cfg config.Config) (*service.Service, error) {
		got = cfg
		return open(cfg)
	}
	deps.Env = func(k string) (string, bool) {
		switch k {
		case "BASHO_UI_DIVISION":
			return "juryo", true
		case "XDG_CONFIG_HOME":
			return t.TempDir(), true
		}
		return "", false
	}
	out, code := run(t, deps, "day", "15", "-t", "202609", "--offline")
	if code != exitOK || !got.Offline || got.UI.Division != "juryo" || !strings.Contains(out, "Juryo · Day 15") {
		t.Errorf("exit %d, offline %v, division %q\n%s", code, got.Offline, got.UI.Division, out)
	}
}
