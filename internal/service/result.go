package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source/resolver"
)

// Result is every use case's answer: the value and where it came from
// (SPEC §5.2).
type Result[T any] struct {
	Value T
	Prov  Provenance
}

// Provenance says where data came from and how fresh it is.
type Provenance struct {
	Source          string     // "snapshot", "sumo-api", "cache:osada", "osada + sumo-api"
	FetchedAt       time.Time  // the snapshot's build time, or the oldest response used
	SourceUpdatedAt *time.Time // osada's resultUpdatedAt
	Stale           bool       // served from an expired cache entry because the source failed
	Forced          bool       // --source chose the source (SPEC §5.4 rule 3)
	Failed          string     // a source that failed before another answered, e.g. "osada"
	FailedErr       string     // its error
}

// SnapshotSource is the provenance label of embedded data.
const SnapshotSource = "snapshot"

// merge combines the provenance of two parts of one answer: the liveliest
// source names it (network, then cache, then snapshot), or both when they
// are as lively; staleness spreads, and the older fetch time counts.
func merge(a, b Provenance) Provenance {
	switch {
	case a.Source == "":
		return b
	case b.Source == "":
		return a
	}
	out := a
	switch la, lb := liveliness(a.Source), liveliness(b.Source); {
	case lb > la:
		out.Source = b.Source
	case lb == la:
		out.Source = union(a.Source, b.Source)
	}
	if b.FetchedAt.Before(out.FetchedAt) {
		out.FetchedAt = b.FetchedAt
	}
	if out.SourceUpdatedAt == nil {
		out.SourceUpdatedAt = b.SourceUpdatedAt
	}
	out.Stale = a.Stale || b.Stale
	out.Forced = a.Forced || b.Forced
	if out.Failed == "" {
		out.Failed, out.FailedErr = b.Failed, b.FailedErr
	}
	return out
}

// union joins two labels of the same liveliness: "sumo-api" and "osada"
// give "sumo-api + osada", "cache:sumo-api" and "cache:osada" give
// "cache:sumo-api + osada".
func union(a, b string) string {
	prefix, a, _ := cutCache(a)
	_, b, _ = cutCache(b)
	names := strings.Split(a, " + ")
	for _, n := range strings.Split(b, " + ") {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	return prefix + strings.Join(names, " + ")
}

func cutCache(s string) (prefix, rest string, ok bool) {
	if rest, ok := strings.CutPrefix(s, "cache:"); ok {
		return "cache:", rest, true
	}
	return "", s, false
}

func liveliness(source string) int {
	switch {
	case source == SnapshotSource:
		return 0
	case strings.HasPrefix(source, "cache:"):
		return 1
	default:
		return 2
	}
}

// Recording wraps the getter under a source adapter used without the
// resolver, so that the service learns, per call, whether the answer came
// from the cache, how old it is and whether it is stale.
func Recording(g httpx.Getter, now func() time.Time) httpx.Getter {
	return resolver.Record(g, "", now)
}

// tracked runs fn against the remote layers and returns its provenance,
// naming the sources the resolver says answered (name without the
// resolver), and the call's record for the per-bout sources.
func tracked(ctx context.Context, name string, now time.Time, fn func(context.Context) error) (Provenance, *resolver.Track, error) {
	ctx, t := resolver.Tracking(ctx)
	err := fn(ctx)
	label, answered := name, t.Answered()
	if len(answered) > 0 {
		label = strings.Join(answered, " + ")
	}
	n, cached, stale, oldest := t.Responses(answered...)
	p := Provenance{Source: label, FetchedAt: now, Stale: stale, Forced: t.Forced()}
	if n > 0 {
		p.FetchedAt = oldest
		if cached == n {
			p.Source = "cache:" + label
		}
	}
	if at := t.UpdatedAt(); !at.IsZero() {
		p.SourceUpdatedAt = &at
	}
	if f := t.Failed(); len(f) > 0 && err == nil {
		p.Failed, p.FailedErr = f[0].Source, f[0].Err.Error()
	}
	return p, t, err
}
