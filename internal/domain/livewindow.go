package domain

import "time"

// LiveWindow is the part of a tournament day when results come in (SPEC
// §10.1), as offsets into the JST day. The final day runs longer for a
// possible playoff.
type LiveWindow struct {
	Start, End, FinalDayEnd time.Duration
}

// DefaultLiveWindow is 14:00–18:45 JST, until 19:15 on day 15.
var DefaultLiveWindow = LiveWindow{
	Start:       14 * time.Hour,
	End:         18*time.Hour + 45*time.Minute,
	FinalDayEnd: 19*time.Hour + 15*time.Minute,
}

// Contains reports whether t falls inside the window of a tournament day.
func (w LiveWindow) Contains(t time.Time, finalDay bool) bool {
	end := w.End
	if finalDay {
		end = w.FinalDayEnd
	}
	at := TimeOfDay(t)
	return at >= w.Start && at < end
}
