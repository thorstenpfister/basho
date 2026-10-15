package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) Env {
	return func(k string) (string, bool) {
		v, ok := vars[k]
		return v, ok
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPrecedence(t *testing.T) {
	path := write(t, `
[ui]
division = "juryo"
win_chance = false

[live]
interval = "5m"
window_start = "13:30"

[sources]
priority_current = ["sumoapi"]
`)
	c, err := Load(path, true, env(map[string]string{
		"BASHO_LIVE_INTERVAL":            "10m", // the environment beats the file
		"BASHO_SOURCE":                   "osada",
		"BASHO_OFFLINE":                  "true",
		"BASHO_SOURCES_PRIORITY_HISTORY": "sumoapi, osada",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.UI.Division != "juryo" || c.UI.WinChance || !c.UI.Animations {
		t.Errorf("ui %+v", c.UI)
	}
	if time.Duration(c.Live.Interval) != 10*time.Minute || c.Live.WindowStart.String() != "13:30" || c.Live.WindowEnd.String() != "18:45" {
		t.Errorf("live %+v", c.Live)
	}
	if c.Source != "osada" || !c.Offline || strings.Join(c.Sources.PriorityHistory, ",") != "sumoapi,osada" || strings.Join(c.Sources.PriorityCurrent, ",") != "sumoapi" {
		t.Errorf("sources %+v, source %q, offline %v", c.Sources, c.Source, c.Offline)
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(write(t, "[ui]\ncolour = \"red\"\n"), false, env(nil)); err == nil || !strings.Contains(err.Error(), "ui.colour") {
		t.Errorf("unknown key: %v", err)
	}
	if _, err := Load(write(t, "[ui\n"), false, env(nil)); err == nil {
		t.Error("broken TOML accepted")
	}
	missing := filepath.Join(t.TempDir(), "none.toml")
	if _, err := Load(missing, true, env(nil)); err == nil {
		t.Error("a required file is missing")
	}
	if c, err := Load(missing, false, env(nil)); err != nil || c.UI.Division != "makuuchi" {
		t.Errorf("an optional missing file: %v", err)
	}
	if _, err := Load("", false, env(map[string]string{"BASHO_UI_ASCII": "maybe"})); err == nil {
		t.Error("a bad boolean in the environment")
	}
	if _, err := Load("", false, env(map[string]string{"BASHO_LIVE_WINDOW_END": "7pm"})); err == nil {
		t.Error("a bad time of day")
	}
}

func TestValidate(t *testing.T) {
	c := Default()
	c.UI.Division = "makushita"
	c.Live.Interval = Duration(time.Minute)
	c.Sources.SumoAPIInterval = Duration(500 * time.Millisecond)
	c.Sources.OsadaInterval = Duration(10 * time.Second)
	c.Sources.PriorityCurrent = []string{"jsa"}
	c.Live.WindowEnd = c.Live.WindowStart
	c.Cache.MaxSizeMB = 0
	err := c.Validate()
	if err == nil {
		t.Fatal("invalid config passed")
	}
	for _, want := range []string{"ui.division", "live.interval = 1m: at least 2m", "sources.sumoapi_interval", "sources.osada_interval", "sources.priority_current", "live.window_end", "cache.max_size_mb"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("errors lack %q:\n%v", want, err)
		}
	}
	stricter := Default()
	stricter.Sources.SumoAPIInterval = Duration(2 * time.Second)
	if err := stricter.Validate(); err != nil {
		t.Errorf("a stricter interval: %v", err)
	}
}

func TestPaths(t *testing.T) {
	if p, _ := DefaultPath(env(map[string]string{"XDG_CONFIG_HOME": "/x"})); p != "/x/basho/config.toml" {
		t.Errorf("config path %s", p)
	}
	if p, _ := DefaultPath(env(nil)); !strings.HasSuffix(p, ".config/basho/config.toml") {
		t.Errorf("config path without XDG %s", p)
	}
	c := Default()
	if d, _ := c.CacheDir(env(map[string]string{"XDG_CACHE_HOME": "/c"})); d != "/c/basho" {
		t.Errorf("cache dir %s", d)
	}
	c.Cache.Dir = "/mine"
	if d, _ := c.CacheDir(env(nil)); d != "/mine" {
		t.Errorf("cache.dir %s", d)
	}
	if d, _ := StateDir(env(map[string]string{"XDG_STATE_HOME": "/s"})); d != "/s/basho" {
		t.Errorf("state dir %s", d)
	}
}

func TestTextForms(t *testing.T) {
	for d, want := range map[time.Duration]string{3 * time.Minute: "3m", 20 * time.Second: "20s", 90 * time.Second: "1m30s", 2 * time.Hour: "2h", 0: "0s"} {
		if b, _ := Duration(d).MarshalText(); string(b) != want {
			t.Errorf("duration %v: %s, want %s", d, b, want)
		}
	}
	if b, _ := TimeOfDay(18*time.Hour + 45*time.Minute).MarshalText(); string(b) != "18:45" {
		t.Errorf("time of day %s", b)
	}
}

// Encode writes what Load reads back, durations as the file writes them.
func TestEncode(t *testing.T) {
	c := Default()
	c.UI.Theme, c.Live.Interval, c.Sources.PriorityCurrent = "light", Duration(90*time.Minute), []string{"sumoapi"}
	text, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[live]\nauto = true\ninterval = \"1h30m\"\n") || !strings.Contains(text, `osada_interval = "20s"`) {
		t.Errorf("encoded:\n%s", text)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path, true, func(string) (string, bool) { return "", false })
	if err != nil || !reflect.DeepEqual(back, c) {
		t.Errorf("read back %+v, %v", back, err)
	}
}
