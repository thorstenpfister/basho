package domain

import "time"

// BashoDays is the length of every tournament in the range basho covers.
const BashoDays = 15

// Held says whether a tournament ran as a regular honbasho. It is calendar
// knowledge; the service maps NotHeld to BashoStatus Cancelled (SPEC §5.7).
type Held int

const (
	HeldRegular Held = iota
	NotHeld          // 2011-03 (match-fixing scandal), 2020-05 (COVID-19)
	HeldSpecial      // 2011-05 technical examination, not an official honbasho
)

var exceptions = map[BashoID]Held{
	{2011, time.March}: NotHeld,
	{2020, time.May}:   NotHeld,
	{2011, time.May}:   HeldSpecial,
}

// Held reports whether b ran as a regular tournament.
func (b BashoID) Held() Held { return exceptions[b] }

// venueEra is a venue used from a given tournament until the next era starts.
type venueEra struct {
	from  BashoID
	venue string
}

// month holds what is fixed per tournament month. venues lists the eras whose
// venue is certain (SPEC Appendix C); before the first era only the city is
// known.
type month struct {
	name, city string
	venues     []venueEra
}

var tokyo = []venueEra{
	{FirstBasho, "Kuramae Kokugikan"},
	{BashoID{1985, time.January}, "Ryogoku Kokugikan"},
}

var months = map[time.Month]month{
	time.January:   {"Hatsu", "Tokyo", tokyo},
	time.March:     {"Haru", "Osaka", []venueEra{{FirstBasho, "Osaka Prefectural Gymnasium"}}},
	time.May:       {"Natsu", "Tokyo", tokyo},
	time.July:      {"Nagoya", "Nagoya", []venueEra{{BashoID{1965, time.July}, "Aichi Prefectural Gymnasium"}, {BashoID{2025, time.July}, "IG Arena"}}},
	time.September: {"Aki", "Tokyo", tokyo},
	time.November:  {"Kyushu", "Fukuoka", []venueEra{{BashoID{1981, time.November}, "Fukuoka Kokusai Center"}}},
}

// Name returns the tournament's name, e.g. "Aki".
func (b BashoID) Name() string { return months[b.Month].name }

// City returns the host city.
func (b BashoID) City() string { return months[b.Month].city }

// Venue returns the arena, or "" where the historical venue is not certain.
// Data from the API takes precedence where it has a location.
func (b BashoID) Venue() string {
	venue := ""
	for _, era := range months[b.Month].venues {
		if b.Compare(era.from) >= 0 {
			venue = era.venue
		}
	}
	return venue
}

// EstimatedStart returns day 1 as the second Sunday of the month, the usual
// rule. The API's start date takes precedence; this is only for tournaments
// whose banzuke is not published yet.
func (b BashoID) EstimatedStart() Date {
	first := NewDate(b.Year, b.Month, 1)
	toSunday := (int(time.Sunday) - int(first.Weekday()) + 7) % 7
	return first.AddDays(toSunday + 7)
}

// UpcomingTournament returns the tournament of d's month, or in an even month
// the next one. The live recorder uses it.
func UpcomingTournament(d Date) BashoID {
	if d.Month%2 == 0 {
		return MostRecentTournament(d).Next()
	}
	return BashoID{d.Year, d.Month}
}

// MostRecentTournament returns the tournament of d's month, or in an even month
// the one before. "current" in a tournament argument means this one.
func MostRecentTournament(d Date) BashoID {
	if d.Month%2 == 0 {
		return BashoID{d.Year, d.Month - 1}
	}
	return BashoID{d.Year, d.Month}
}

// DayDate returns the date of tournament day n (1-based) for a tournament that
// starts on start.
func DayDate(start Date, n int) Date { return start.AddDays(n - 1) }
