package service

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// Open builds the production service from the configuration: the embedded
// snapshot (unless NoSnapshot) and calibrated rating, then sumo-api over the
// disk cache and the polite HTTP client (cache only when Offline).
func Open(cfg config.Config, version string, env config.Env) (*Service, error) {
	if cfg.Source == "osada" {
		return nil, fmt.Errorf("--source osada: the osada.us adapter comes with the current-tournament support (Phase 9)")
	}
	dir, err := cfg.CacheDir(env)
	if err != nil {
		return nil, err
	}
	c, err := cache.Open(dir, int64(cfg.Cache.MaxSizeMB)<<20)
	if err != nil {
		return nil, err
	}
	var log *slog.Logger
	if cfg.Debug {
		if log, err = debugLog(env); err != nil {
			return nil, err
		}
	}
	u, err := url.Parse(sumoapi.DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	client := httpx.New(httpx.Options{
		UserAgent: httpx.UserAgent(version),
		Hosts: map[string]httpx.HostPolicy{
			u.Host: {Interval: time.Duration(cfg.Sources.SumoAPIInterval), Burst: 3, MaxConcurrent: 2},
		},
		Log: log,
	})
	policy := sumoapi.Freshness{
		LiveInterval: time.Duration(cfg.Live.Interval),
		Window: domain.LiveWindow{ // day 15 runs 30 minutes longer (SPEC §12)
			Start: time.Duration(cfg.Live.WindowStart), End: time.Duration(cfg.Live.WindowEnd),
			FinalDayEnd: time.Duration(cfg.Live.WindowEnd) + 30*time.Minute,
		},
	}
	getter := Recording(&cache.Getter{Cache: c, Next: client, Policy: policy.Policy, Offline: cfg.Offline}, time.Now)
	api := sumoapi.New(getter, sumoapi.DefaultBaseURL)
	opts := Options{Snapshot: data.Snapshot, Params: data.RatingParams, Remote: api, Wrestlers: api, RemoteName: "sumo-api"}
	if cfg.NoSnapshot {
		opts.Snapshot = nil
	}
	return New(opts), nil
}

// debugLog appends to $XDG_STATE_HOME/basho/debug.log (SPEC §7.1).
func debugLog(env config.Env) (*slog.Logger, error) {
	dir, err := config.StateDir(env)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "debug.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})), nil
}
