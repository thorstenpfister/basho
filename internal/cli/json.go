package cli

import (
	"encoding/json"
	"io"
	"math"
	"strings"
	"time"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
)

// The --json output (SPEC §11, docs/json.md). These types are the published
// schema: they are converted from the service's views, so internal changes
// do not change the output. Golden files lock them down.

// jsonSchema is the envelope's schemaVersion.
const jsonSchema = 1

type envelope struct {
	SchemaVersion int            `json:"schemaVersion"`
	Kind          string         `json:"kind"`
	Provenance    jsonProvenance `json:"provenance"`
	Data          any            `json:"data"`
}

type jsonProvenance struct {
	Source          string     `json:"source"`
	FetchedAt       time.Time  `json:"fetchedAt"`
	SourceUpdatedAt *time.Time `json:"sourceUpdatedAt"`
	Stale           bool       `json:"stale"`
}

func writeJSON(w io.Writer, kind string, p service.Provenance, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(envelope{
		SchemaVersion: jsonSchema, Kind: kind, Data: data,
		Provenance: jsonProvenance{Source: p.Source, FetchedAt: p.FetchedAt.UTC(), SourceUpdatedAt: p.SourceUpdatedAt, Stale: p.Stale},
	})
}

// lower is the stable lowercase name of an enum.
func lower(s string) string { return strings.ToLower(s) }

type jsonRank struct {
	Title  string `json:"title"`  // yokozuna … juryo, makushita
	Number int    `json:"number"` // 1-based
	Side   string `json:"side"`   // east | west
	Short  string `json:"short"`  // "M17e"
}

func rankJSON(r domain.Rank) *jsonRank {
	if r.Title == 0 {
		return nil
	}
	return &jsonRank{Title: lower(r.Title.String()), Number: r.Number, Side: lower(r.Side.String()), Short: r.Short()}
}

type jsonName struct {
	En string `json:"en"`
	Jp string `json:"jp,omitempty"`
}

type jsonWrestler struct {
	ID      domain.RikishiID `json:"id"`
	Shikona jsonName         `json:"shikona"`
	Rank    *jsonRank        `json:"rank,omitempty"`
}

func wrestlerJSON(w service.Wrestler) jsonWrestler {
	return jsonWrestler{ID: w.RikishiID, Shikona: jsonName{En: w.Shikona.En, Jp: w.Shikona.Jp}, Rank: rankJSON(w.Rank)}
}

type jsonRecord struct {
	Wins     int    `json:"wins"`
	Losses   int    `json:"losses"`
	Absences int    `json:"absences"`
	Draws    int    `json:"draws"`
	Text     string `json:"text"` // "5-7-3"
}

func recordJSON(r derive.Record) jsonRecord {
	return jsonRecord{Wins: r.Wins, Losses: r.Losses, Absences: r.Absences, Draws: r.Draws, Text: r.String()}
}

type jsonTechnique struct {
	Kimarite string `json:"kimarite"`
	Kanji    string `json:"kanji,omitempty"`
	English  string `json:"english,omitempty"`
}

func techniqueJSON(t service.Technique) *jsonTechnique {
	if t.Kimarite == "" {
		return nil
	}
	return &jsonTechnique{Kimarite: string(t.Kimarite), Kanji: t.Kanji, English: t.English}
}

// side is "east", "west" or null for an undecided bout.
func side(s domain.Side) *string {
	if s == 0 {
		return nil
	}
	v := lower(s.String())
	return &v
}

// day

type jsonDay struct {
	Basho    domain.BashoID `json:"basho"`
	Name     string         `json:"name"`
	Venue    string         `json:"venue"`
	Division string         `json:"division"`
	Day      int            `json:"day"`
	Date     domain.Date    `json:"date"`
	Status   string         `json:"status"` // not_published | scheduled | in_progress | complete
	Bouts    []jsonBoutRow  `json:"bouts"`
	Playoffs []jsonBoutRow  `json:"playoffs"`
	Kyujo    []jsonWrestler `json:"kyujo"`
}

type jsonBoutRow struct {
	MatchNo   int            `json:"matchNo"`
	East      jsonBoutSide   `json:"east"`
	West      jsonBoutSide   `json:"west"`
	Winner    *string        `json:"winner"`
	Technique *jsonTechnique `json:"technique"`
	Fusen     bool           `json:"fusen"`
	Kinboshi  bool           `json:"kinboshi"`
	Musubi    bool           `json:"musubi"`
	WinChance *float64       `json:"winChance"` // the east side's chance before the bout; null when the rating is hidden
}

type jsonBoutSide struct {
	jsonWrestler
	Before *jsonRecord `json:"before"` // null for a visitor from a lower division
	After  *jsonRecord `json:"after"`
}

func dayJSON(d service.Day) jsonDay {
	rows := func(rs []service.BoutRow) []jsonBoutRow {
		out := make([]jsonBoutRow, 0, len(rs))
		for _, r := range rs {
			bs := func(s service.BoutSide) jsonBoutSide {
				out := jsonBoutSide{jsonWrestler: wrestlerJSON(s.Wrestler)}
				if !s.Visitor {
					before, after := recordJSON(s.Before), recordJSON(s.After)
					out.Before, out.After = &before, &after
				}
				return out
			}
			out = append(out, jsonBoutRow{
				MatchNo: r.MatchNo, East: bs(r.East), West: bs(r.West), Winner: side(r.Winner),
				Technique: techniqueJSON(r.Kimarite), Fusen: r.Fusen, Kinboshi: r.Kinboshi, Musubi: r.Musubi, WinChance: round(r.WinChance),
			})
		}
		return out
	}
	kyujo := make([]jsonWrestler, 0, len(d.Kyujo))
	for _, w := range d.Kyujo {
		kyujo = append(kyujo, wrestlerJSON(w))
	}
	return jsonDay{
		Basho: d.Basho, Name: d.Name, Venue: d.Venue, Division: lower(d.Division.String()), Day: d.Day, Date: d.Date,
		Status: d.Status.String(), Bouts: rows(d.Bouts), Playoffs: rows(d.Playoffs), Kyujo: kyujo,
	}
}

// round keeps 4 decimals of a probability, so golden files do not depend on
// the last bits of floating point.
func round(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := roundTo(*p, 4)
	return &v
}

func roundTo(v float64, places int) float64 {
	f := math.Pow(10, float64(places))
	return math.Round(v*f) / f
}

// bout

type jsonTale struct {
	Basho      domain.BashoID  `json:"basho"`
	Name       string          `json:"name"`
	Division   string          `json:"division"`
	Day        int             `json:"day"`
	MatchNo    int             `json:"matchNo"`
	Of         int             `json:"of"`
	Musubi     bool            `json:"musubi"`
	Playoff    bool            `json:"playoff"`
	East       jsonTaleSide    `json:"east"`
	West       jsonTaleSide    `json:"west"`
	HeadToHead jsonHeadToHead  `json:"headToHead"`
	Matchup    *jsonMatchup    `json:"matchup"`
	WinChance  *float64        `json:"winChance"`
	Result     *jsonTaleResult `json:"result"` // null while undecided
}

type jsonTaleSide struct {
	jsonWrestler
	Heya     string         `json:"heya"`
	Shusshin string         `json:"shusshin"`
	Profile  bool           `json:"profile"` // false: no profile, the next fields are empty
	Age      int            `json:"age"`
	Height   float64        `json:"height"`
	Weight   float64        `json:"weight"`
	Latest   bool           `json:"measurementsLatest"` // height and weight are the latest known, not the tournament's
	Now      string         `json:"now,omitempty"`      // today's shikona when it changed
	Yusho    map[string]int `json:"yusho"`
	Sansho   map[string]int `json:"sansho"`
	Record   *jsonRecord    `json:"record"` // null for a visitor from a lower division
	Last5    jsonLast       `json:"last5"`
	Kinboshi jsonKinboshi   `json:"kinboshi"`
	Moveset  jsonMoveset    `json:"moveset"`
	Rating   *jsonRating    `json:"rating"`
}

type jsonLast struct {
	Results []jsonResult `json:"results"` // oldest first
	New     bool         `json:"new"`
}

type jsonResult struct {
	Basho    domain.BashoID   `json:"basho"`
	Day      int              `json:"day"`
	Opponent domain.RikishiID `json:"opponent"`
	Won      bool             `json:"won"`
	Fusen    bool             `json:"fusen"`
	Kimarite string           `json:"kimarite"`
}

type jsonKinboshi struct {
	Count     int `json:"count"`
	ThisBasho int `json:"thisBasho"`
}

type jsonMoveset struct {
	Window         string             `json:"window"` // last 6 basho | career | this basho
	Tournaments    int                `json:"tournaments"`
	Wins           int                `json:"wins"`
	Losses         int                `json:"losses"`
	WinShare       map[string]float64 `json:"winShare"` // by category: push, belt, throw, pull, other
	LossShare      map[string]float64 `json:"lossShare"`
	TopWins        []jsonCount        `json:"topWins"`
	TopLosses      []jsonCount        `json:"topLosses"`
	Signature      *jsonCount         `json:"signature"`
	SignatureShare float64            `json:"signatureShare"`
	FighterType    string             `json:"fighterType"` // empty below 10 wins
	Enough         bool               `json:"enough"`
}

type jsonCount struct {
	Kimarite string `json:"kimarite"`
	Count    int    `json:"count"`
}

type jsonRating struct {
	Value       float64 `json:"value"`
	Change      float64 `json:"change"` // since the tournament's start
	Provisional bool    `json:"provisional"`
}

type jsonHeadToHead struct {
	EastWins int           `json:"eastWins"`
	WestWins int           `json:"westWins"`
	EastTop  []jsonCount   `json:"eastTop"`
	WestTop  []jsonCount   `json:"westTop"`
	Meetings []jsonMeeting `json:"meetings"` // newest first
}

type jsonMeeting struct {
	Basho    domain.BashoID   `json:"basho"`
	Day      int              `json:"day"`
	Winner   domain.RikishiID `json:"winner"`
	Kimarite string           `json:"kimarite"`
	EastRank *jsonRank        `json:"eastRank"`
	WestRank *jsonRank        `json:"westRank"`
	Playoff  bool             `json:"playoff"`
}

type jsonMatchup struct {
	Attacker  string  `json:"attacker"` // east | west
	Category  string  `json:"category"`
	WinShare  float64 `json:"winShare"`
	LossShare float64 `json:"lossShare"`
}

type jsonTaleResult struct {
	Winner    string        `json:"winner"`
	Technique jsonTechnique `json:"technique"`
	Kinboshi  bool          `json:"kinboshi"`
	Fusen     bool          `json:"fusen"`
	Upset     bool          `json:"upset"`
	East      jsonRecord    `json:"east"` // after the bout
	West      jsonRecord    `json:"west"`
}

func countsJSON(top []derive.KimariteCount) []jsonCount {
	out := make([]jsonCount, 0, len(top))
	for _, k := range top {
		out = append(out, jsonCount{Kimarite: string(k.Kimarite), Count: k.Count})
	}
	return out
}

func shares(s derive.Shares) map[string]float64 {
	m := map[string]float64{}
	for _, c := range categories {
		m[c.String()] = roundTo(s[c], 4)
	}
	return m
}

func taleSideJSON(t service.TaleSide) jsonTaleSide {
	yusho := map[string]int{}
	for d, n := range t.Career.Yusho {
		yusho[lower(d.String())] = n
	}
	last := jsonLast{Results: []jsonResult{}, New: t.Last5.New}
	for _, r := range t.Last5.Results {
		last.Results = append(last.Results, jsonResult{Basho: r.Basho, Day: r.Day, Opponent: r.Opponent, Won: r.Won, Fusen: r.Fusen, Kimarite: string(r.Kimarite)})
	}
	m := t.Moveset
	mv := jsonMoveset{
		Window: m.Window.String(), Tournaments: m.Tournaments, Wins: m.Wins, Losses: m.Losses,
		WinShare: shares(m.WinShare), LossShare: shares(m.LossShare), TopWins: countsJSON(m.TopWins), TopLosses: countsJSON(m.TopLosses),
		SignatureShare: roundTo(m.SignatureShare, 4), FighterType: m.Type.String(), Enough: m.Enough(),
	}
	if m.Signature.Count > 0 {
		mv.Signature = &jsonCount{Kimarite: string(m.Signature.Kimarite), Count: m.Signature.Count}
	}
	out := jsonTaleSide{
		jsonWrestler: wrestlerJSON(t.Wrestler), Heya: t.Heya, Shusshin: t.Shusshin, Profile: t.Profile,
		Age: t.Career.Age, Height: t.Career.Height, Weight: t.Career.Weight, Latest: t.Career.Latest, Now: t.Career.Now,
		Yusho: yusho, Sansho: t.Career.Sansho, Last5: last,
		Kinboshi: jsonKinboshi{Count: t.Kinboshi.Count, ThisBasho: t.Kinboshi.ThisBasho}, Moveset: mv,
	}
	if !t.Visitor {
		r := recordJSON(t.Record)
		out.Record = &r
	}
	if t.Rating != nil {
		out.Rating = &jsonRating{Value: roundTo(t.Rating.Value, 2), Change: roundTo(t.Rating.Change, 2), Provisional: t.Rating.Provisional}
	}
	return out
}

func taleJSON(t service.Tale) jsonTale {
	h := t.HeadToHead
	hh := jsonHeadToHead{EastWins: h.A.Wins, WestWins: h.B.Wins, EastTop: countsJSON(h.A.Top), WestTop: countsJSON(h.B.Top), Meetings: []jsonMeeting{}}
	for _, m := range h.Meetings {
		hh.Meetings = append(hh.Meetings, jsonMeeting{
			Basho: m.Basho, Day: m.Day, Winner: m.Winner, Kimarite: string(m.Kimarite),
			EastRank: rankJSON(m.RankA), WestRank: rankJSON(m.RankB), Playoff: m.Playoff,
		})
	}
	out := jsonTale{
		Basho: t.Basho, Name: t.Name, Division: lower(t.Division.String()), Day: t.Day, MatchNo: t.MatchNo, Of: t.Of,
		Musubi: t.Musubi, Playoff: t.Playoff, East: taleSideJSON(t.East), West: taleSideJSON(t.West),
		HeadToHead: hh, WinChance: round(t.WinChance),
	}
	if m := t.Matchup; m != nil {
		att := "west"
		if m.AttackerIsA {
			att = "east"
		}
		out.Matchup = &jsonMatchup{Attacker: att, Category: m.Category.String(), WinShare: roundTo(m.WinShare, 4), LossShare: roundTo(m.LossShare, 4)}
	}
	if r := t.Result; r != nil {
		out.Result = &jsonTaleResult{
			Winner: lower(r.Winner.String()), Technique: *techniqueJSON(r.Technique), Kinboshi: r.Kinboshi, Fusen: r.Fusen,
			Upset: r.WinnerUpset, East: recordJSON(r.East), West: recordJSON(r.West),
		}
	}
	return out
}

// standings

type jsonStandings struct {
	Basho        domain.BashoID `json:"basho"`
	Name         string         `json:"name"`
	Division     string         `json:"division"`
	Day          int            `json:"day"`
	Rows         []jsonStanding `json:"rows"` // in standings order; sort by rank for the grid
	YushoDecided bool           `json:"yushoDecided"`
	Playoff      bool           `json:"playoff"`
	Yusho        *jsonWrestler  `json:"yusho"` // once the tournament is final
	KachiKoshi   int            `json:"kachiKoshi"`
	MakeKoshi    int            `json:"makeKoshi"`
	Kyujo        int            `json:"kyujo"`
}

type jsonStanding struct {
	jsonWrestler
	Record       jsonRecord `json:"record"`
	Remaining    int        `json:"remaining"`
	Band         int        `json:"band"` // wins behind the leader, 3 for the rest
	InContention bool       `json:"inContention"`
	KachiKoshi   bool       `json:"kachiKoshi"`
	MakeKoshi    bool       `json:"makeKoshi"`
	NewToday     bool       `json:"newToday"`
	Kyujo        bool       `json:"kyujo"`
	Results      []string   `json:"results"` // 15 outcomes; pending after the day
}

func standingsJSON(s service.Standings) jsonStandings {
	out := jsonStandings{
		Basho: s.Basho, Name: s.Name, Division: lower(s.Division.String()), Day: s.Day, Rows: []jsonStanding{},
		YushoDecided: s.YushoDecided, Playoff: s.Playoff, KachiKoshi: s.KachiKoshi, MakeKoshi: s.MakeKoshi, Kyujo: s.Kyujo,
	}
	if s.Yusho != nil {
		w := wrestlerJSON(*s.Yusho)
		out.Yusho = &w
	}
	for _, r := range s.Rows {
		res := make([]string, len(r.Results))
		for i, d := range r.Results {
			res[i] = d.Outcome.String()
		}
		out.Rows = append(out.Rows, jsonStanding{
			jsonWrestler: wrestlerJSON(service.Wrestler{RikishiID: r.RikishiID, Shikona: r.Shikona, Rank: r.Rank}),
			Record:       recordJSON(r.Record), Remaining: r.Remaining, Band: int(r.Band), InContention: r.InContention,
			KachiKoshi: r.Record.KachiKoshi(), MakeKoshi: r.Record.MakeKoshi(), NewToday: r.NewToday, Kyujo: r.Kyujo, Results: res,
		})
	}
	return out
}

// info and list

type jsonInfo struct {
	Basho  domain.BashoID `json:"basho"`
	Name   string         `json:"name"`
	Venue  string         `json:"venue"`
	Status string         `json:"status"` // upcoming | in_progress | finished | cancelled
	Start  *domain.Date   `json:"start"`
	End    *domain.Date   `json:"end"`
	Yusho  []jsonWinner   `json:"yusho"`
	Prizes []jsonWinner   `json:"prizes"`
}

type jsonWinner struct {
	Division string `json:"division,omitempty"`
	Prize    string `json:"prize,omitempty"`
	jsonWrestler
	Record *jsonRecord `json:"record"` // final record; null for a lower division's winner
}

func winnerJSON(w service.Winner) jsonWinner {
	out := jsonWinner{Division: lower(w.Division.String()), Prize: w.Prize, jsonWrestler: wrestlerJSON(w.Wrestler)}
	if w.Record != nil {
		r := recordJSON(*w.Record)
		out.Record = &r
	}
	return out
}

func infoJSON(in service.Info) jsonInfo {
	out := jsonInfo{Basho: in.Basho.ID, Name: in.Name, Venue: in.Venue, Status: in.Status.String(), Yusho: []jsonWinner{}, Prizes: []jsonWinner{}}
	if !in.Basho.Start.IsZero() {
		out.Start, out.End = &in.Basho.Start, &in.Basho.End
	}
	for _, y := range in.Yusho {
		out.Yusho = append(out.Yusho, winnerJSON(y))
	}
	for _, p := range in.Prizes {
		out.Prizes = append(out.Prizes, winnerJSON(p))
	}
	return out
}

type jsonListEntry struct {
	Basho      domain.BashoID `json:"basho"`
	Name       string         `json:"name"`
	Venue      string         `json:"venue"`
	Status     string         `json:"status"`
	Yusho      *jsonWinner    `json:"yusho"`
	InSnapshot bool           `json:"inSnapshot"`
}

func listJSON(es []service.ListEntry) []jsonListEntry {
	out := make([]jsonListEntry, 0, len(es))
	for _, e := range es {
		j := jsonListEntry{Basho: e.Basho, Name: e.Name, Venue: e.Venue, Status: e.Status.String(), InSnapshot: e.InSnapshot}
		if e.Yusho != nil {
			w := winnerJSON(*e.Yusho)
			j.Yusho = &w
		}
		out = append(out, j)
	}
	return out
}

// ratings

type jsonRatings struct {
	Basho domain.BashoID   `json:"basho"`
	Shown bool             `json:"shown"` // false: the rating did not pass calibration (V9)
	Rows  []jsonRatingsRow `json:"rows"`
}

type jsonRatingsRow struct {
	jsonWrestler
	jsonRating
}

func ratingsJSON(t service.RatingTable) jsonRatings {
	out := jsonRatings{Basho: t.Basho, Shown: t.Shown, Rows: []jsonRatingsRow{}}
	for _, r := range t.Rows {
		out.Rows = append(out.Rows, jsonRatingsRow{
			jsonWrestler: wrestlerJSON(service.Wrestler{RikishiID: r.RikishiID, Shikona: r.Shikona, Rank: r.Rank}),
			jsonRating:   jsonRating{Value: roundTo(r.Value, 2), Change: roundTo(r.Change, 2), Provisional: r.Provisional},
		})
	}
	return out
}
