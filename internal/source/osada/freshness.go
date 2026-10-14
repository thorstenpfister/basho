package osada

import (
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Freshness says how long osada.us's files stay fresh in the cache (SPEC
// §7.2); its Policy method is a cache.Policy. osada.us only ever serves the
// current tournament, so nothing is pinned: revalidating is a cheap 304
// thanks to its ETag.
type Freshness struct {
	LiveInterval time.Duration     // inside the live window; zero means 3 min
	Window       domain.LiveWindow // zero means domain.DefaultLiveWindow
}

// ttlQuiet is the TTL outside the live window: osada.us publishes the next
// day's bouts a few times a day (SPEC §4.2).
const ttlQuiet = 30 * time.Minute

// Policy implements cache.Policy for osada.us URLs.
func (f Freshness) Policy(_ string, _ int, _ []byte, now time.Time) (time.Duration, bool) {
	if f.LiveInterval == 0 {
		f.LiveInterval = 3 * time.Minute
	}
	if f.Window == (domain.LiveWindow{}) {
		f.Window = domain.DefaultLiveWindow
	}
	if f.Window.Contains(now, true) { // the files do not say which day is the last; the longer window is harmless
		return f.LiveInterval, false
	}
	return ttlQuiet, false
}
