package config

import (
	"os"
	"path/filepath"
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
	for _, want := range []string{"ui.division", "live.interval = 1m0s: at least 2m0s", "sources.sumoapi_interval", "sources.osada_interval", "sources.priority_current", "live.window_end", "cache.max_size_mb"} {
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
	if b, _ := Duration(3 * time.Minute).MarshalText(); string(b) != "3m0s" {
		t.Errorf("duration %s", b)
	}
	if b, _ := TimeOfDay(18*time.Hour + 45*time.Minute).MarshalText(); string(b) != "18:45" {
		t.Errorf("time of day %s", b)
	}
}
