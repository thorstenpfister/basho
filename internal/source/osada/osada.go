package osada

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source"
)

// DefaultBaseURL is osada.us, which serves the current tournament as static JSON.
const DefaultBaseURL = "https://osada.us"

// Paths of the two files basho reads (SPEC §4.2).
const (
	TorikumiPath = "/api/v1/torikumi.json"
	BanzukePath  = "/api/v1/banzuke.json"
)

// Client reads osada.us.
type Client struct {
	http httpx.Getter
	base string
}

// New returns a Client for base (DefaultBaseURL in production).
func New(g httpx.Getter, base string) *Client {
	return &Client{http: g, base: base}
}

var _ source.CurrentSource = (*Client)(nil)

// Current returns the current tournament: two requests, the banzuke and the
// whole bout list. Between tournaments the banzuke can be newer than the
// bout list (the next tournament's ranking is out); its days are then left
// out.
func (c *Client) Current(ctx context.Context) (source.Current, error) {
	b, err := c.bodies(ctx)
	if err != nil {
		return source.Current{}, err
	}
	return mapCurrent(b.banzuke, b.torikumi)
}

type bodies struct {
	banzuke  banzukeDTO
	torikumi torikumiDTO
}

func (c *Client) bodies(ctx context.Context) (bodies, error) {
	var b bodies
	if err := c.getJSON(ctx, BanzukePath, &b.banzuke); err != nil {
		return b, err
	}
	return b, c.getJSON(ctx, TorikumiPath, &b.torikumi)
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	resp, err := c.http.Get(ctx, c.base+path, httpx.Validators{})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("osada %s: %w: %w", path, source.ErrUpstream, err)
	}
	switch {
	case resp.OK():
	case resp.Status == http.StatusNotFound:
		return fmt.Errorf("osada %s: %w", path, source.ErrNotFound)
	case resp.Status == http.StatusTooManyRequests:
		return fmt.Errorf("osada %s: %w", path, source.ErrRateLimited)
	default:
		return fmt.Errorf("osada %s: HTTP %d: %w", path, resp.Status, source.ErrUpstream)
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		return fmt.Errorf("osada %s: %w: %w", path, source.ErrUpstream, err)
	}
	return nil
}

func mapCurrent(b banzukeDTO, t torikumiDTO) (source.Current, error) {
	id, err := parseBasho(b.Year, b.BashoName)
	if err != nil {
		return source.Current{}, fmt.Errorf("osada banzuke: %w: %w", source.ErrBadData, err)
	}
	out := source.Current{Basho: id, Banzuke: map[domain.Division][]source.CurrentEntry{}}
	out.UpdatedAt = parseTime(b.UpdatedAt)
	ranks := map[int]domain.Rank{}
	for _, div := range []struct {
		div  domain.Division
		rows []banzukeRow
	}{{domain.DivMakuuchi, b.Makuuchi}, {domain.DivJuryo, b.Juryo}} {
		entries, err := mapBanzuke(div.rows)
		if err != nil {
			return source.Current{}, fmt.Errorf("osada banzuke %s: %w: %w", div.div, source.ErrBadData, err)
		}
		for _, e := range entries {
			ranks[e.JSAID] = e.Rank
		}
		out.Banzuke[div.div] = entries
	}
	if len(ranks) == 0 {
		return source.Current{}, fmt.Errorf("osada banzuke %s: %w", id, source.ErrNotYetAvailable)
	}

	days, tid, err := mapDays(t, ranks)
	if err != nil {
		return source.Current{}, fmt.Errorf("osada torikumi: %w: %w", source.ErrBadData, err)
	}
	if tid != id { // the bout list is still the previous tournament's
		return out, nil
	}
	out.Days = days
	if len(days) > 0 {
		out.Start = days[0].Date.AddDays(1 - days[0].Day) // day 1, even when day 1 is missing
	}
	out.ResultUpdatedAt, out.ScheduleUpdatedAt = parseTime(t.ResultUpdatedAt), parseTime(t.ScheduleUpdatedAt)
	if u := parseTime(t.UpdatedAt); u.After(out.UpdatedAt) {
		out.UpdatedAt = u
	}
	return out, nil
}

// mapBanzuke reads a division's rows. A row is one rank level; within a
// side, wrestlers of an unnumbered title (two Ozeki East) count up from 1.
func mapBanzuke(rows []banzukeRow) ([]source.CurrentEntry, error) {
	var out []source.CurrentEntry
	for _, row := range rows {
		level, err := parseRank(row.Title)
		if err != nil {
			return nil, err
		}
		for _, side := range []struct {
			side domain.Side
			list []rikishiDTO
		}{{domain.East, row.East}, {domain.West, row.West}} {
			for i, r := range side.list {
				rank := level
				rank.Side = side.side
				if level.Title.Sanyaku() { // written without a number
					rank.Number = i + 1
				}
				e := source.CurrentEntry{JSAID: r.ID, Name: domain.Name{Jp: r.Name, Kana: r.Yomi}, Rank: rank}
				if len(r.Results) > domain.BashoDays {
					return nil, fmt.Errorf("%s: %d results", r.Name, len(r.Results))
				}
				for d, res := range r.Results {
					if e.Results[d], err = outcome(res); err != nil {
						return nil, fmt.Errorf("%s day %d: %w", r.Name, d+1, err)
					}
				}
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// outcome maps a banzuke result. osada.us writes absences as "draw" (a
// wrestler kyujo for the whole tournament has 15 of them); there are no
// draws in the modern era.
func outcome(s *string) (domain.Outcome, error) {
	if s == nil {
		return domain.Pending, nil
	}
	switch *s {
	case "win":
		return domain.Win, nil
	case "loss":
		return domain.Loss, nil
	case "draw":
		return domain.Absent, nil
	}
	return 0, fmt.Errorf("unknown result %q", *s)
}

// mapDays merges the result and schedule lists: a day's results when they
// are published, otherwise its schedule. It returns the tournament the days
// belong to.
func mapDays(t torikumiDTO, ranks map[int]domain.Rank) ([]source.CurrentDay, domain.BashoID, error) {
	byDay := map[int]dayDTO{}
	for _, d := range t.ScheduleDays {
		if d.Status == "published" && d.Data != nil {
			byDay[d.Day] = d
		}
	}
	for _, d := range t.ResultDays {
		if d.Status == "published" && d.Data != nil {
			byDay[d.Day] = d
		}
	}
	var id domain.BashoID
	var out []source.CurrentDay
	for n := 1; n <= domain.BashoDays; n++ {
		d, ok := byDay[n]
		if !ok {
			continue
		}
		date, err := domain.ParseDate(d.ISODate)
		if err != nil {
			return nil, id, fmt.Errorf("day %d: %w", n, err)
		}
		if len(d.PathDate) < 6 {
			return nil, id, fmt.Errorf("day %d: pathDate %q", n, d.PathDate)
		}
		if id, err = domain.ParseBashoID(d.PathDate[:6]); err != nil {
			return nil, id, fmt.Errorf("day %d: %w", n, err)
		}
		day := source.CurrentDay{Day: n, Date: date, Bouts: map[domain.Division][]source.CurrentBout{}, Absentees: map[domain.Division][]int{}}
		for _, div := range []struct {
			div domain.Division
			dto *divisionDTO
		}{{domain.DivMakuuchi, d.Data.Makuuchi}, {domain.DivJuryo, d.Data.Juryo}} {
			if div.dto == nil {
				continue
			}
			for _, m := range div.dto.Matches {
				b, err := mapBout(m, ranks)
				if err != nil {
					return nil, id, fmt.Errorf("day %d %s bout %d: %w", n, div.div, m.BoutNo, err)
				}
				day.Bouts[div.div] = append(day.Bouts[div.div], b)
			}
			for _, a := range div.dto.Absentees {
				day.Absentees[div.div] = append(day.Absentees[div.div], a.ID)
			}
		}
		out = append(out, day)
	}
	return out, id, nil
}

func mapBout(m matchDTO, ranks map[int]domain.Rank) (source.CurrentBout, error) {
	b := source.CurrentBout{MatchNo: m.BoutNo, IsPlayoff: m.IsPlayoff}
	var err error
	if b.East, err = mapSide(m.EastName, m.EastRank, m.EastProfileURL, ranks); err != nil {
		return b, err
	}
	if b.West, err = mapSide(m.WestName, m.WestRank, m.WestProfileURL, ranks); err != nil {
		return b, err
	}
	k := kimarite(m.Kimarite)
	if m.Winner == nil || k == "" {
		return b, nil // undecided
	}
	switch *m.Winner {
	case "east":
		b.Winner = domain.East
	case "west":
		b.Winner = domain.West
	default:
		return b, fmt.Errorf("winner %q", *m.Winner)
	}
	b.Kimarite = k
	return b, nil
}

// mapSide takes the rank from the banzuke, which has the side; a visitor
// from Makushita keeps the bout list's rank, without one.
func mapSide(name, rank, profile string, ranks map[int]domain.Rank) (source.CurrentSide, error) {
	id, err := JSAID(profile)
	if err != nil {
		return source.CurrentSide{}, err
	}
	s := source.CurrentSide{JSAID: id, Kana: name}
	if r, ok := ranks[id]; ok {
		s.Rank = r
		return s, nil
	}
	if s.Rank, err = parseRank(rank); err != nil {
		return source.CurrentSide{}, err
	}
	return s, nil
}

var profileID = regexp.MustCompile(`/profile/(\d+)/?$`)

// JSAID reads the JSA ID from a profile URL,
// "https://www.sumo.or.jp/ResultRikishiData/profile/3553/".
func JSAID(url string) (int, error) {
	m := profileID.FindStringSubmatch(url)
	if m == nil {
		return 0, fmt.Errorf("profile URL %q: no JSA ID", url)
	}
	return strconv.Atoi(m[1])
}

// parseTime reads osada.us's ISO times ("2026-09-27T17:27:38+09:00"); bad or
// empty is zero.
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
