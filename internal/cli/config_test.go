package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/fang"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cache"
)

// runAll is run with stderr too.
func runAll(t *testing.T, deps Deps, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := NewRoot(buildinfo.Info{Version: "devel"}, deps)
	var out, errs bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errs)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		errs.WriteString(err.Error())
	}
	return out.String(), errs.String(), exitCode(err)
}

func TestConfigPath(t *testing.T) {
	deps := fixtureDeps(t)
	dir, _ := deps.Env("XDG_CONFIG_HOME")
	want := filepath.Join(dir, "basho", "config.toml")
	out, errs, code := runAll(t, deps, "config", "path")
	if code != exitOK || out != want+"\n" || !strings.Contains(errs, "No file there yet") {
		t.Errorf("exit %d, %q, %q", code, out, errs)
	}
	// A broken file can still be found.
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("[ui\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, errs, code := runAll(t, deps, "config", "path"); code != exitOK || out != want+"\n" || errs != "" {
		t.Errorf("broken file: exit %d, %q, %q", code, out, errs)
	}
	out, _, _ = runAll(t, deps, "config", "path", "--json")
	var env struct {
		Kind string `json:"kind"`
		Data struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Kind != "config_path" || env.Data.Path != want || !env.Data.Exists {
		t.Errorf("--json %+v, %v:\n%s", env, err, out)
	}
	// A broken file fails with where settings come from.
	if _, errs, code := runAll(t, deps, "config", "show"); code != exitUsage || !strings.Contains(errs, "Settings come from "+want) {
		t.Errorf("show over a broken file: exit %d, %q", code, errs)
	}
}

func TestConfigShow(t *testing.T) {
	deps := fixtureDeps(t)
	dir, _ := deps.Env("XDG_CONFIG_HOME")
	path := filepath.Join(dir, "basho", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[live]\ninterval = \"5m\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := deps.Env
	deps.Env = func(k string) (string, bool) {
		if k == "BASHO_UI_THEME" {
			return "light", true
		}
		return env(k)
	}
	out, _, code := runAll(t, deps, "config", "show", "--ascii")
	for _, want := range []string{
		"# file   " + path + "\n",
		"# this run: source = auto, offline = false, debug = false",
		"[ui]\ndivision = \"makuuchi\"\n",
		"ascii = true  # default: false\n",                   // flag
		"theme = \"light\"  # default: \"auto\"\n",           // environment
		"interval = \"5m\"  # default: \"3m\"\n",             // file
		"window_start = \"14:00\"\nwindow_end = \"18:45\"\n", // defaults, unmarked
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exit %d, lacks %q:\n%s", code, want, out)
		}
	}
	out, _, _ = runAll(t, deps, "config", "show", "--json")
	var envl struct {
		Kind string                    `json:"kind"`
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envl); err != nil || envl.Kind != "config" || envl.Data["live"]["interval"] != "5m" || envl.Data["ui"]["theme"] != "light" {
		t.Errorf("--json %+v, %v", envl, err)
	}
}

// Failures say what to do on a line of their own.
func TestHints(t *testing.T) {
	deps := fixtureDeps(t)
	cause := fmt.Errorf("https://www.sumo-api.com/api/basho/202611: %w", cache.ErrOffline)
	offline := &app{}
	offline.cfg.Offline = true
	if err := offline.explain(cause); exitCode(err) != exitUnavailable || err.Error() != "not in the snapshot or the cache, and --offline is set\nRun it once without --offline to fetch and cache it" {
		t.Errorf("offline: %v", err)
	}
	if err := (&app{}).explain(cause); exitCode(err) != exitUnavailable || !strings.Contains(err.Error(), "sumo-api.com") || !strings.Contains(err.Error(), "\nCheck the connection") {
		t.Errorf("online: %v", err)
	}
	dir, _ := deps.Env("XDG_STATE_HOME")
	if err := os.MkdirAll(filepath.Join(dir, "basho"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "basho", "state.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errs, _ := runAll(t, deps, "day", "13", "-t", "202609"); !strings.Contains(errs, "delete it to start afresh") {
		t.Errorf("state: %q", errs)
	}
}

// The error box keeps paths as they are: fang capitalised the first word
// (/Private/Tmp/…) and wrapped at hyphens.
func TestErrorHandler(t *testing.T) {
	var b bytes.Buffer
	path := "/private/tmp/claude-501/a-long-directory-name-with-hyphens/basho/state.json"
	errorHandler(&b, fang.Styles{ErrorText: lipgloss.NewStyle().Width(20).Transform(strings.ToUpper)}, errors.New(path+": bad"))
	if !strings.Contains(b.String(), path+": bad.") {
		t.Errorf("%q", b.String())
	}
}
