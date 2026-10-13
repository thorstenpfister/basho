package domain

import "time"

// JST is Japan Standard Time. Japan has used a constant UTC+9 without daylight
// saving since 1951, so a fixed zone is exact for every tournament basho
// covers (1960 on) and needs no time zone database.
var JST = time.FixedZone("JST", 9*60*60)

// TimeOfDay returns how far t is into its Japanese calendar day, for
// comparisons like "between 14:00 and 18:45 JST".
func TimeOfDay(t time.Time) time.Duration {
	return t.Sub(DateOf(t).Time())
}
