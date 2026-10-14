package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is basho's configuration (SPEC §12). Precedence: flags > environment
// (BASHO_*) > file > defaults.
type Config struct {
	UI       UI       `toml:"ui"`
	Spoilers Spoilers `toml:"spoilers"`
	Live     Live     `toml:"live"`
	Sources  Sources  `toml:"sources"`
	Cache    Cache    `toml:"cache"`
	Prefetch Prefetch `toml:"prefetch"`

	// Session switches: flags or environment only, never the file.
	Source     string `toml:"-"` // auto | sumoapi | osada (BASHO_SOURCE)
	Offline    bool   `toml:"-"` // snapshot and cache only (BASHO_OFFLINE)
	NoSnapshot bool   `toml:"-"` // development: ignore the embedded snapshot
	Debug      bool   `toml:"-"` // debug log (BASHO_DEBUG)
}

// UI settings.
type UI struct {
	Division        string `toml:"division"` // makuuchi | juryo
	Names           string `toml:"names"`    // both | en | jp
	ASCII           bool   `toml:"ascii"`
	Animations      bool   `toml:"animations"`
	Mouse           bool   `toml:"mouse"`
	Theme           string `toml:"theme"` // auto | dark | light
	WinChance       bool   `toml:"win_chance"`
	WinChanceColumn bool   `toml:"win_chance_column"`
	Portraits       bool   `toml:"portraits"`
}

// Spoilers settings.
type Spoilers struct {
	Mode string `toml:"mode"` // show | hide
}

// Live mode settings.
type Live struct {
	Auto        bool      `toml:"auto"`
	Interval    Duration  `toml:"interval"`     // at least MinLiveInterval
	WindowStart TimeOfDay `toml:"window_start"` // JST
	WindowEnd   TimeOfDay `toml:"window_end"`
}

// Sources settings. The intervals can only be made stricter than the
// built-in limits (SPEC §7.1).
type Sources struct {
	PriorityHistory []string `toml:"priority_history"`
	PriorityCurrent []string `toml:"priority_current"`
	OsadaEnabled    bool     `toml:"osada_enabled"`
	SumoAPIInterval Duration `toml:"sumoapi_interval"`
	OsadaInterval   Duration `toml:"osada_interval"`
}

// Cache settings.
type Cache struct {
	Dir       string `toml:"dir"` // empty: the platform cache directory
	MaxSizeMB int    `toml:"max_size_mb"`
}

// Prefetch settings.
type Prefetch struct {
	AdjacentDays bool `toml:"adjacent_days"`
}

// The built-in limits: configuration may only be stricter.
const (
	MinLiveInterval = 2 * time.Minute
	MinSumoAPI      = time.Second
	MinOsada        = 20 * time.Second
)

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		UI:       UI{Division: "makuuchi", Names: "both", Animations: true, Mouse: true, Theme: "auto", WinChance: true, Portraits: true},
		Spoilers: Spoilers{Mode: "show"},
		Live: Live{
			Auto: true, Interval: Duration(3 * time.Minute),
			WindowStart: TimeOfDay(14 * time.Hour), WindowEnd: TimeOfDay(18*time.Hour + 45*time.Minute),
		},
		Sources: Sources{
			PriorityHistory: []string{"sumoapi"}, PriorityCurrent: []string{"osada", "sumoapi"}, OsadaEnabled: true,
			SumoAPIInterval: Duration(MinSumoAPI), OsadaInterval: Duration(MinOsada),
		},
		Cache:  Cache{MaxSizeMB: 200},
		Source: "auto",
	}
}

// Env looks up an environment variable; os.LookupEnv in production.
type Env func(string) (string, bool)

// DefaultPath returns $XDG_CONFIG_HOME/basho/config.toml, otherwise
// ~/.config/basho/config.toml, on every platform (SPEC §12).
func DefaultPath(env Env) (string, error) {
	if dir, ok := env("XDG_CONFIG_HOME"); ok && dir != "" {
		return filepath.Join(dir, "basho", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "basho", "config.toml"), nil
}

// CacheDir returns the cache directory: cache.dir, $XDG_CACHE_HOME/basho, or
// the platform's (~/Library/Caches/basho on macOS).
func (c Config) CacheDir(env Env) (string, error) {
	if c.Cache.Dir != "" {
		return c.Cache.Dir, nil
	}
	if dir, ok := env("XDG_CACHE_HOME"); ok && dir != "" {
		return filepath.Join(dir, "basho"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "basho"), nil
}

// StateDir returns $XDG_STATE_HOME/basho, otherwise ~/.local/state/basho:
// the debug log and spoiler state live there.
func StateDir(env Env) (string, error) {
	if dir, ok := env("XDG_STATE_HOME"); ok && dir != "" {
		return filepath.Join(dir, "basho"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Local", "basho"), nil
	}
	return filepath.Join(home, ".local", "state", "basho"), nil
}

// Load reads the configuration: defaults, then the file at path, then the
// environment. A missing file is fine unless required (an explicit
// --config). Flags are applied by the caller, then Validate.
func Load(path string, required bool, env Env) (Config, error) {
	c := Default()
	if path != "" {
		md, err := toml.DecodeFile(path, &c)
		switch {
		case errors.Is(err, fs.ErrNotExist) && !required:
		case err != nil:
			return Config{}, fmt.Errorf("config %s: %w", path, err)
		default:
			if undecoded := md.Undecoded(); len(undecoded) > 0 {
				return Config{}, fmt.Errorf("config %s: unknown setting %s", path, undecoded[0])
			}
		}
	}
	if err := c.applyEnv(env); err != nil {
		return Config{}, err
	}
	return c, nil
}

// applyEnv sets every setting from BASHO_<SECTION>_<KEY> (for example
// BASHO_LIVE_INTERVAL=5m), and the session switches from BASHO_SOURCE,
// BASHO_OFFLINE and BASHO_DEBUG. Lists are comma separated.
func (c *Config) applyEnv(env Env) error {
	root := reflect.ValueOf(c).Elem()
	for i := range root.NumField() {
		section := root.Type().Field(i)
		name, _, _ := strings.Cut(section.Tag.Get("toml"), ",")
		if name == "-" {
			continue
		}
		for j := range section.Type.NumField() {
			key, _, _ := strings.Cut(section.Type.Field(j).Tag.Get("toml"), ",")
			variable := "BASHO_" + strings.ToUpper(name+"_"+key)
			if v, ok := env(variable); ok {
				if err := set(root.Field(i).Field(j), v); err != nil {
					return fmt.Errorf("%s: %w", variable, err)
				}
			}
		}
	}
	for variable, field := range map[string]any{"BASHO_SOURCE": &c.Source, "BASHO_OFFLINE": &c.Offline, "BASHO_DEBUG": &c.Debug} {
		if v, ok := env(variable); ok {
			if err := set(reflect.ValueOf(field).Elem(), v); err != nil {
				return fmt.Errorf("%s: %w", variable, err)
			}
		}
	}
	return nil
}

func set(f reflect.Value, v string) error {
	if u, ok := f.Addr().Interface().(interface{ UnmarshalText([]byte) error }); ok {
		return u.UnmarshalText([]byte(v))
	}
	switch f.Kind() {
	case reflect.String:
		f.SetString(v)
	case reflect.Bool:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return err
		}
		f.SetBool(b)
	case reflect.Int:
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		f.SetInt(int64(n))
	case reflect.Slice:
		var items []string
		for item := range strings.SplitSeq(v, ",") {
			if item = strings.TrimSpace(item); item != "" {
				items = append(items, item)
			}
		}
		f.Set(reflect.ValueOf(items))
	default:
		return fmt.Errorf("unsupported setting type %s", f.Type())
	}
	return nil
}

// Validate checks every setting, including that intervals are not looser
// than the built-in limits.
func (c Config) Validate() error {
	var errs []error
	oneOf := func(name, v string, allowed ...string) {
		if !slices.Contains(allowed, v) {
			errs = append(errs, fmt.Errorf("%s = %q: want one of %s", name, v, strings.Join(allowed, ", ")))
		}
	}
	atLeast := func(name string, d Duration, limit time.Duration) {
		if time.Duration(d) < limit {
			errs = append(errs, fmt.Errorf("%s = %s: at least %s", name, time.Duration(d), limit))
		}
	}
	oneOf("ui.division", c.UI.Division, "makuuchi", "juryo")
	oneOf("ui.names", c.UI.Names, "both", "en", "jp")
	oneOf("ui.theme", c.UI.Theme, "auto", "dark", "light")
	oneOf("spoilers.mode", c.Spoilers.Mode, "show", "hide")
	oneOf("source", c.Source, "auto", "sumoapi", "osada")
	atLeast("live.interval", c.Live.Interval, MinLiveInterval)
	atLeast("sources.sumoapi_interval", c.Sources.SumoAPIInterval, MinSumoAPI)
	atLeast("sources.osada_interval", c.Sources.OsadaInterval, MinOsada)
	if c.Live.WindowEnd <= c.Live.WindowStart {
		errs = append(errs, fmt.Errorf("live.window_end %s is not after live.window_start %s", c.Live.WindowEnd, c.Live.WindowStart))
	}
	for name, list := range map[string][]string{"sources.priority_history": c.Sources.PriorityHistory, "sources.priority_current": c.Sources.PriorityCurrent} {
		if len(list) == 0 {
			errs = append(errs, fmt.Errorf("%s is empty", name))
		}
		for _, v := range list {
			oneOf(name, v, "sumoapi", "osada")
		}
	}
	if c.Cache.MaxSizeMB <= 0 {
		errs = append(errs, fmt.Errorf("cache.max_size_mb = %d: must be positive", c.Cache.MaxSizeMB))
	}
	slices.SortFunc(errs, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	return errors.Join(errs...)
}

// Duration is a time.Duration written as "3m" in the file.
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	*d = Duration(v)
	return err
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// TimeOfDay is a JST time of day written as "14:00".
type TimeOfDay time.Duration

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *TimeOfDay) UnmarshalText(b []byte) error {
	v, err := time.Parse("15:04", string(b))
	if err != nil {
		return fmt.Errorf("time of day %q: want HH:MM", b)
	}
	*t = TimeOfDay(time.Duration(v.Hour())*time.Hour + time.Duration(v.Minute())*time.Minute)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (t TimeOfDay) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (t TimeOfDay) String() string {
	d := time.Duration(t)
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}
