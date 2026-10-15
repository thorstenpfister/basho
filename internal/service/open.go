package service

import (
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
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/resolver"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// Open builds the production service from the configuration: the embedded
// snapshot (unless NoSnapshot) and calibrated rating, then the resolver over
// sumo-api and osada.us, each over the disk cache and the polite HTTP client
// (cache only when Offline).
func Open(cfg config.Config, version string, env config.Env) (*Service, error) {
	mode, err := resolver.ParseMode(cfg.Source)
	if err != nil {
		return nil, err
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
	sumoHost, err := host(sumoapi.DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	osadaHost, err := host(osada.DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	client := httpx.New(httpx.Options{
		UserAgent: httpx.UserAgent(version),
		Hosts: map[string]httpx.HostPolicy{
			sumoHost:  {Interval: time.Duration(cfg.Sources.SumoAPIInterval), Burst: 3, MaxConcurrent: 2},
			osadaHost: {Interval: time.Duration(cfg.Sources.OsadaInterval), Burst: 2, MaxConcurrent: 1},
		},
		Log: log,
	})
	interval := time.Duration(cfg.Live.Interval)
	window := domain.LiveWindow{ // day 15 runs 30 minutes longer (SPEC §12)
		Start: time.Duration(cfg.Live.WindowStart), End: time.Duration(cfg.Live.WindowEnd),
		FinalDayEnd: time.Duration(cfg.Live.WindowEnd) + 30*time.Minute,
	}
	cached := func(name string, policy cache.Policy) httpx.Getter {
		return resolver.Record(&cache.Getter{Cache: c, Next: client, Policy: policy, Offline: cfg.Offline}, name, time.Now)
	}
	api := sumoapi.New(cached(resolver.SumoAPI, sumoapi.Freshness{LiveInterval: interval, Window: window}.Policy), sumoapi.DefaultBaseURL)
	res := &resolver.Resolver{Historical: api, Mode: mode, Log: log}
	if cfg.Sources.OsadaEnabled || mode == resolver.ForceOsada {
		res.Current = osada.New(cached(resolver.Osada, osada.Freshness{LiveInterval: interval, Window: window}.Policy), osada.DefaultBaseURL)
	}
	opts := Options{Snapshot: data.Snapshot, Params: data.RatingParams, Portraits: data.Portraits, Remote: res, Wrestlers: api, RemoteName: resolver.SumoAPI}
	if cfg.NoSnapshot {
		opts.Snapshot = nil
	}
	svc := New(opts)
	res.IDs = svc.IDs
	return svc, nil
}

func host(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	return u.Host, nil
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
