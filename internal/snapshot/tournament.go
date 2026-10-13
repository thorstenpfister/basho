package snapshot

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Tournament is everything the snapshot holds about one finished tournament.
type Tournament struct {
	Basho   domain.Basho
	Banzuke map[domain.Division][]domain.BanzukeEntry
	Bouts   []domain.Bout // in SortBouts order once decoded
}

// RikishiIDs returns everyone on the banzuke or in a bout, including
// visitors from Makushita.
func (t Tournament) RikishiIDs() []domain.RikishiID {
	seen := map[domain.RikishiID]bool{}
	for _, entries := range t.Banzuke {
		for _, e := range entries {
			seen[e.RikishiID] = true
		}
	}
	for _, b := range t.Bouts {
		seen[b.East.RikishiID], seen[b.West.RikishiID] = true, true
	}
	return slices.Sorted(maps.Keys(seen))
}

// SortBouts puts bouts in the snapshot's canonical order: by day, division,
// regular bouts before playoffs, then match number.
func SortBouts(bouts []domain.Bout) {
	slices.SortStableFunc(bouts, compareBouts)
}

func compareBouts(a, b domain.Bout) int {
	return cmp.Or(cmp.Compare(a.Day, b.Day), cmp.Compare(a.Division, b.Division),
		boolCompare(a.IsPlayoff, b.IsPlayoff), cmp.Compare(a.MatchNo, b.MatchNo))
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// The file stores each fact once (SPEC §4.3, V8):
//
//   - A bout side is only the wrestler's id when its romaji shikona and rank
//     are those on the banzuke and it has no kanji or kana (sumo-api's bout
//     lists carry romaji only); visitors and other differences are written
//     out.
//   - A banzuke record is one letter per day. Opponent and kimarite come from
//     that day's bout; only days where they differ are written out.
//
// Decoding restores exactly what was encoded.

type tournamentFile struct {
	versioned
	ID       domain.BashoID                       `json:"id"`
	Start    domain.Date                          `json:"start"`
	End      domain.Date                          `json:"end"`
	Location string                               `json:"location,omitempty"`
	Yusho    map[domain.Division]domain.RikishiID `json:"yusho"`
	Prizes   []prizeFile                          `json:"prizes,omitempty"`
	Banzuke  map[domain.Division][]entryFile      `json:"banzuke"`
	Bouts    []boutFile                           `json:"bouts"`
}

type prizeFile struct {
	Type      string           `json:"type"`
	RikishiID domain.RikishiID `json:"id"`
}

type entryFile struct {
	ID     domain.RikishiID `json:"id"`
	En     string           `json:"en"`
	Jp     string           `json:"jp,omitempty"`
	Kana   string           `json:"kana,omitempty"`
	Rank   domain.Rank      `json:"rank"`
	Record string           `json:"rec"`         // one outcome letter per day
	Except []exceptionFile  `json:"x,omitempty"` // days not described by their bout
}

type exceptionFile struct {
	Day      int              `json:"d"`
	Opponent domain.RikishiID `json:"o,omitempty"`
	Kimarite domain.Kimarite  `json:"k,omitempty"`
}

type boutFile struct {
	Division domain.Division `json:"div"`
	Day      int             `json:"d"`
	MatchNo  int             `json:"n"`
	East     sideFile        `json:"e"`
	West     sideFile        `json:"w"`
	Winner   domain.Side     `json:"win,omitempty"` // 1 east, 2 west
	Kimarite domain.Kimarite `json:"k,omitempty"`
	Playoff  bool            `json:"p,omitempty"`
}

// sideFile is a bout side; the name and rank are omitted when they are the
// banzuke's romaji name and rank.
type sideFile struct {
	ID   domain.RikishiID `json:"id"`
	En   string           `json:"en,omitempty"`
	Jp   string           `json:"jp,omitempty"`
	Kana string           `json:"kana,omitempty"`
	Rank domain.Rank      `json:"r,omitzero"`
}

// outcomeLetters encodes domain.Outcome by its value.
const outcomeLetters = ".WLFfADU" // pending win loss fusen-win fusen-loss absent draw unknown

// EncodeTournament returns a tournament file's content: compact JSON, zstd
// compressed. The same tournament always encodes to the same bytes, so a
// rebuild leaves git unchanged.
func EncodeTournament(t Tournament) ([]byte, error) {
	bouts := slices.Clone(t.Bouts)
	SortBouts(bouts)
	ranked := banzukeSides(t.Banzuke)
	dayBouts := BoutsByDay(bouts)

	f := tournamentFile{
		versioned: versioned{Schema},
		ID:        t.Basho.ID, Start: t.Basho.Start, End: t.Basho.End, Location: t.Basho.Location,
		Yusho:   t.Basho.Yusho,
		Banzuke: map[domain.Division][]entryFile{},
	}
	for _, p := range t.Basho.Prizes {
		f.Prizes = append(f.Prizes, prizeFile{p.Type, p.RikishiID})
	}
	for div, entries := range t.Banzuke {
		for _, e := range entries {
			ef := entryFile{ID: e.RikishiID, En: e.Shikona.En, Jp: e.Shikona.Jp, Kana: e.Shikona.Kana, Rank: e.Rank}
			var rec strings.Builder
			for i, r := range e.Results {
				if int(r.Outcome) >= len(outcomeLetters) {
					return nil, fmt.Errorf("%s: wrestler %d day %d: outcome %d", t.Basho.ID, e.RikishiID, i+1, r.Outcome)
				}
				rec.WriteByte(outcomeLetters[r.Outcome])
				if opp, k := dayBouts.Opponent(e.RikishiID, i+1); opp != r.OpponentID || k != r.Kimarite {
					ef.Except = append(ef.Except, exceptionFile{i + 1, r.OpponentID, r.Kimarite})
				}
			}
			ef.Record = rec.String()
			f.Banzuke[div] = append(f.Banzuke[div], ef)
		}
	}
	for _, b := range bouts {
		f.Bouts = append(f.Bouts, boutFile{
			Division: b.Division, Day: b.Day, MatchNo: b.MatchNo,
			East: ranked.compact(b.East), West: ranked.compact(b.West),
			Winner: b.Winner, Kimarite: b.Kimarite, Playoff: b.IsPlayoff,
		})
	}
	return encode(f)
}

// DecodeTournament parses a tournament file.
func DecodeTournament(b []byte) (Tournament, error) {
	var f tournamentFile
	if err := decode(b, &f); err != nil {
		return Tournament{}, err
	}
	if err := f.check(); err != nil {
		return Tournament{}, fmt.Errorf("%s: %w", f.ID, err)
	}
	t := Tournament{
		Basho: domain.Basho{
			ID: f.ID, Start: f.Start, End: f.End, Location: f.Location, Yusho: f.Yusho,
		},
		Banzuke: make(map[domain.Division][]domain.BanzukeEntry, len(f.Banzuke)),
		Bouts:   make([]domain.Bout, 0, len(f.Bouts)),
	}
	if t.Basho.Yusho == nil {
		t.Basho.Yusho = map[domain.Division]domain.RikishiID{}
	}
	for _, p := range f.Prizes {
		t.Basho.Prizes = append(t.Basho.Prizes, domain.Prize{Type: p.Type, RikishiID: p.RikishiID})
	}
	ranked := sides{}
	for _, entries := range f.Banzuke {
		for _, ef := range entries {
			ranked[ef.ID] = domain.BoutSide{RikishiID: ef.ID, Shikona: domain.Name{En: ef.En, Jp: ef.Jp, Kana: ef.Kana}, Rank: ef.Rank}
		}
	}
	for _, bf := range f.Bouts {
		t.Bouts = append(t.Bouts, domain.Bout{
			Basho: f.ID, Division: bf.Division, Day: bf.Day, MatchNo: bf.MatchNo,
			East: ranked.full(bf.East), West: ranked.full(bf.West),
			Winner: bf.Winner, Kimarite: bf.Kimarite, IsPlayoff: bf.Playoff,
		})
	}
	dayBouts := BoutsByDay(t.Bouts)
	for div, entries := range f.Banzuke {
		out := make([]domain.BanzukeEntry, 0, len(entries))
		for _, ef := range entries {
			if len(ef.Record) != domain.BashoDays {
				return Tournament{}, fmt.Errorf("%s: wrestler %d: record %q", f.ID, ef.ID, ef.Record)
			}
			e := domain.BanzukeEntry{RikishiID: ef.ID, Shikona: domain.Name{En: ef.En, Jp: ef.Jp, Kana: ef.Kana}, Rank: ef.Rank}
			for i := range domain.BashoDays {
				o := strings.IndexByte(outcomeLetters, ef.Record[i])
				if o < 0 {
					return Tournament{}, fmt.Errorf("%s: wrestler %d: record %q", f.ID, ef.ID, ef.Record)
				}
				opp, k := dayBouts.Opponent(ef.ID, i+1)
				e.Results[i] = domain.DayResult{Outcome: domain.Outcome(o), OpponentID: opp, Kimarite: k}
			}
			for _, x := range ef.Except {
				if x.Day < 1 || x.Day > domain.BashoDays {
					return Tournament{}, fmt.Errorf("%s: wrestler %d: exception for day %d", f.ID, ef.ID, x.Day)
				}
				e.Results[x.Day-1].OpponentID, e.Results[x.Day-1].Kimarite = x.Opponent, x.Kimarite
			}
			out = append(out, e)
		}
		t.Banzuke[div] = out
	}
	return t, nil
}

// sides holds each banzuke wrestler as a bout side.
type sides map[domain.RikishiID]domain.BoutSide

func banzukeSides(banzuke map[domain.Division][]domain.BanzukeEntry) sides {
	s := sides{}
	for _, entries := range banzuke {
		for _, e := range entries {
			s[e.RikishiID] = domain.BoutSide{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank}
		}
	}
	return s
}

func (s sides) compact(b domain.BoutSide) sideFile {
	if known, ok := s[b.RikishiID]; ok && b == romaji(known) {
		return sideFile{ID: b.RikishiID}
	}
	return sideFile{b.RikishiID, b.Shikona.En, b.Shikona.Jp, b.Shikona.Kana, b.Rank}
}

func (s sides) full(f sideFile) domain.BoutSide {
	if known, ok := s[f.ID]; ok && f.En == "" && f.Jp == "" && f.Kana == "" && f.Rank == (domain.Rank{}) {
		return romaji(known)
	}
	return domain.BoutSide{RikishiID: f.ID, Shikona: domain.Name{En: f.En, Jp: f.Jp, Kana: f.Kana}, Rank: f.Rank}
}

// romaji is a banzuke side as a bout list writes it: romaji name and rank.
func romaji(b domain.BoutSide) domain.BoutSide {
	return domain.BoutSide{RikishiID: b.RikishiID, Shikona: domain.Name{En: b.Shikona.En}, Rank: b.Rank}
}

// DayIndex finds a wrestler's regular (non-playoff) bout of a day.
type DayIndex map[dayKey]domain.Bout

type dayKey struct {
	id  domain.RikishiID
	day int
}

// BoutsByDay indexes bouts by wrestler and day.
func BoutsByDay(bouts []domain.Bout) DayIndex {
	idx := DayIndex{}
	for _, b := range bouts {
		if b.IsPlayoff {
			continue
		}
		idx[dayKey{b.East.RikishiID, b.Day}] = b
		idx[dayKey{b.West.RikishiID, b.Day}] = b
	}
	return idx
}

// Bout returns id's regular bout of day.
func (idx DayIndex) Bout(id domain.RikishiID, day int) (domain.Bout, bool) {
	b, ok := idx[dayKey{id, day}]
	return b, ok
}

// Opponent returns what a banzuke record says by default for a day: the
// opponent and kimarite of that day's bout.
func (idx DayIndex) Opponent(id domain.RikishiID, day int) (domain.RikishiID, domain.Kimarite) {
	b, ok := idx[dayKey{id, day}]
	switch {
	case !ok:
		return 0, ""
	case b.East.RikishiID == id:
		return b.West.RikishiID, b.Kimarite
	default:
		return b.East.RikishiID, b.Kimarite
	}
}
