package source

import (
	"context"
	"errors"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

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

// CurrentSource serves the current tournament in one piece (SPEC §5.3):
// osada.us.
type CurrentSource interface {
	Current(ctx context.Context) (Current, error)
}

// Current is the current tournament as osada.us serves it: both divisions'
// banzuke and every published day. Wrestlers are identified by their JSA ID
// (sumo-api's nskId); the resolver maps them to sumo-api IDs (SPEC §5.5).
type Current struct {
	Basho             domain.BashoID
	Start             domain.Date // day 1; zero before the schedule is out
	UpdatedAt         time.Time
	ResultUpdatedAt   time.Time // zero when unknown
	ScheduleUpdatedAt time.Time
	Banzuke           map[domain.Division][]CurrentEntry
	Days              []CurrentDay // published days, day 1 first
}

// CurrentEntry is a wrestler on the current banzuke.
type CurrentEntry struct {
	JSAID int
	Name  domain.Name // Jp and Kana; En only if the source has it
	Rank  domain.Rank
	// Results is the banzuke's tally per day: Win, Loss, Absent or Pending.
	// It does not tell fusen apart; the day's bouts do.
	Results [domain.BashoDays]domain.Outcome
}

// CurrentDay is one published day of both divisions.
type CurrentDay struct {
	Day       int
	Date      domain.Date
	Bouts     map[domain.Division][]CurrentBout
	Absentees map[domain.Division][]int // JSA IDs
}

// CurrentBout is one bout: the winner and kimarite are zero until decided.
type CurrentBout struct {
	MatchNo    int
	East, West CurrentSide
	Winner     domain.Side
	Kimarite   domain.Kimarite
	IsPlayoff  bool
}

// CurrentSide is one wrestler of a bout.
type CurrentSide struct {
	JSAID int
	Kana  string      // the bout list's name, in hiragana
	Rank  domain.Rank // from the banzuke; visitors' from the bout list, without a side
}
