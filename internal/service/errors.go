package service

import (
	"errors"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source"
)

// NotFound reports a failure because the data does not exist or not yet: a
// future tournament, an unpublished day, a bout number out of range. The CLI
// exits with 3 (SPEC §11).
func NotFound(err error) bool {
	return errors.Is(err, source.ErrNotFound) || errors.Is(err, source.ErrNotYetAvailable)
}

// Unavailable reports a failure because every source failed and nothing is
// cached (also offline without a cached copy). The CLI exits with 4.
func Unavailable(err error) bool {
	return errors.Is(err, source.ErrUpstream) || errors.Is(err, source.ErrRateLimited) ||
		errors.Is(err, cache.ErrOffline) || errors.Is(err, httpx.ErrCircuitOpen)
}
