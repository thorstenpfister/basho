package source

import "errors"

// Errors every adapter maps its failures to (SPEC §5.3). Adapters wrap them,
// so callers test with errors.Is.
var (
	ErrNotFound        = errors.New("not found")
	ErrNotYetAvailable = errors.New("not available yet") // e.g. a tournament whose banzuke is not published
	ErrUpstream        = errors.New("upstream error")    // transport failure, 5xx, unparseable data
	ErrRateLimited     = errors.New("rate limited")      // 429
	// ErrBadData is a response that arrived but cannot be mapped (an unknown
	// rank, an impossible winner). Unlike ErrUpstream, retrying will not help.
	ErrBadData = errors.New("unmappable data")
)
