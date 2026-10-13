package sumoapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Freshness classifies sumo-api responses into the data classes of SPEC §7.2.
// Its Policy method is a cache.Policy.
type Freshness struct {
	LiveInterval time.Duration     // today's results during the live window; zero means 3 min
	Window       domain.LiveWindow // zero means domain.DefaultLiveWindow
}

// TTLs per data class (SPEC §7.2).
const (
	ttlPastDay  = 30 * time.Minute   // current tournament, an earlier day
	ttlCurrent  = time.Hour          // current tournament outside the live window
	ttlIndex    = 7 * 24 * time.Hour // the wrestler index
	ttlNotFound = 6 * time.Hour      // a tournament sumo-api does not have yet
	defaultLive = 3 * time.Minute    // the live interval of SPEC §10.2
)

// Policy implements cache.Policy for sumo-api URLs.
//
// A tournament is finished, and its responses pinned, once its end date plus
// two days has passed and a yusho winner is present. Metadata and bout lists
// carry the tournament's dates and yusho; banzuke responses do not, so they
// go by the estimated end date alone.
func (f Freshness) Policy(rawURL string, status int, body []byte, now time.Time) (time.Duration, bool) {
	if f.LiveInterval == 0 {
		f.LiveInterval = defaultLive
	}
	if f.Window == (domain.LiveWindow{}) {
		f.Window = domain.DefaultLiveWindow
	}
	if status == http.StatusNotFound {
		return ttlNotFound, false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ttlCurrent, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/") // api basho 202611 torikumi Makuuchi 3
	switch {
	case len(parts) >= 2 && parts[1] == "rikishis":
		return ttlIndex, false
	case len(parts) < 3 || parts[1] != "basho":
		return ttlCurrent, false
	}
	id, err := domain.ParseBashoID(parts[2])
	if err != nil {
		return ttlCurrent, false
	}
	today := domain.DateOf(now)

	if len(parts) >= 4 && parts[3] == "banzuke" {
		if today.DaysSince(id.EstimatedEnd()) >= domain.FinalMargin {
			return 0, true
		}
		return f.current(now, false), false
	}

	var dto struct { // only what the class depends on
		Date      string            `json:"date"`
		StartDate time.Time         `json:"startDate"`
		EndDate   time.Time         `json:"endDate"`
		Yusho     []json.RawMessage `json:"yusho"`
	}
	if json.Unmarshal(body, &dto) != nil {
		return ttlCurrent, false
	}
	if dto.Date == "" { // the zero object: not known yet
		return ttlNotFound, false
	}
	start, end := domain.DateOf(dto.StartDate), domain.DateOf(dto.EndDate)
	if domain.IsFinal(end, len(dto.Yusho) > 0, today) {
		return 0, true
	}
	if len(parts) == 6 && parts[3] == "torikumi" {
		day, err := strconv.Atoi(parts[5])
		if err == nil && today.Compare(domain.DayDate(start, day)) > 0 {
			return ttlPastDay, false
		}
	}
	return f.current(now, today == end), false
}

// current is the TTL for the current tournament: the live interval inside the
// window, an hour outside it.
func (f Freshness) current(now time.Time, finalDay bool) time.Duration {
	if !f.Window.Contains(now, finalDay) {
		return ttlCurrent
	}
	return f.LiveInterval
}
