package osada

import (
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestFreshness(t *testing.T) {
	day := func(h, m int) time.Time { return time.Date(2026, time.November, 10, h, m, 0, 0, domain.JST) }
	for _, tt := range []struct {
		now  time.Time
		want time.Duration
	}{
		{day(16, 0), 3 * time.Minute},
		{day(10, 0), ttlQuiet},
		{day(23, 0), ttlQuiet},
	} {
		ttl, pinned := Freshness{}.Policy(DefaultBaseURL+TorikumiPath, 200, nil, tt.now)
		if ttl != tt.want || pinned {
			t.Errorf("%s: %s, pinned %v; want %s", tt.now.Format("15:04"), ttl, pinned, tt.want)
		}
	}
	if ttl, _ := (Freshness{}).Policy(DefaultBaseURL+BanzukePath, 200, nil, day(16, 0)); ttl != ttlQuiet {
		t.Errorf("banzuke in the window: %s", ttl)
	}
	if ttl, _ := (Freshness{LiveInterval: 5 * time.Minute}).Policy("", 200, nil, day(16, 0)); ttl != 5*time.Minute {
		t.Errorf("configured interval: %s", ttl)
	}
}
