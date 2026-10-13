package domain

// RikishiID identifies a wrestler by sumo-api.com's id.
type RikishiID int

// Kimarite is a winning technique by its canonical key, e.g. "yorikiri"
// (SPEC Appendix B).
type Kimarite string

// Outcome is one wrestler's result on one day.
type Outcome int

const (
	Pending Outcome = iota // not decided yet
	Win
	Loss
	FusenWin  // won by default: the opponent did not appear
	FusenLoss // lost by default
	Absent    // kyujo
	Draw      // hikiwake, old era only
	Unknown   // a source value basho does not recognise
)

var outcomeNames = [...]string{"pending", "win", "loss", "fusen_win", "fusen_loss", "absent", "draw", "unknown"}

// String returns the stable lowercase name, as used in JSON (SPEC §11).
func (o Outcome) String() string {
	if o < 0 || int(o) >= len(outcomeNames) {
		return "unknown"
	}
	return outcomeNames[o]
}

// MarshalText implements encoding.TextMarshaler, so JSON carries the name.
func (o Outcome) MarshalText() ([]byte, error) { return []byte(o.String()), nil }

// IsWin reports a win, including by default.
func (o Outcome) IsWin() bool { return o == Win || o == FusenWin }

// IsLoss reports a loss, including by default.
func (o Outcome) IsLoss() bool { return o == Loss || o == FusenLoss }

// Name is a shikona as of a tournament.
type Name struct {
	En   string // romaji, e.g. "Onosato"
	Jp   string // kanji, cleaned (SPEC §5.6), e.g. "大の里"; empty for many old records
	Kana string // reading, e.g. "おおのさと"; only from osada.us
}

// DayResult is one day of a wrestler's banzuke record.
type DayResult struct {
	Outcome    Outcome
	OpponentID RikishiID // zero when absent
	Kimarite   Kimarite
}

// BanzukeEntry is one wrestler on a tournament's banzuke.
type BanzukeEntry struct {
	RikishiID RikishiID
	Shikona   Name
	Rank      Rank
	Results   [BashoDays]DayResult // index = day-1
}

// BoutSide is one wrestler in a bout.
type BoutSide struct {
	RikishiID RikishiID
	Shikona   Name
	Rank      Rank
}

// Bout is one match of a day.
type Bout struct {
	Basho      BashoID
	Division   Division
	Day        int // 1..15
	MatchNo    int // 1-based display order
	East, West BoutSide
	Winner     Side // zero: not decided yet
	Kimarite   Kimarite
	IsPlayoff  bool
}

// Rikishi is a wrestler's profile.
type Rikishi struct {
	ID       RikishiID
	NSKID    int // Japan Sumo Association id; osada.us uses it (SPEC §5.5)
	SumoDBID int
	Name     Name // current
	Heya     string
	Shusshin string // hometown
	Birth    Date
	Debut    BashoID
	Height   float64 // cm, current
	Weight   float64 // kg, current
}
