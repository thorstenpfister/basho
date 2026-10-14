package bout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// fixtureService is the fixture snapshot (196001, 202607, 202609), stable
// across snapshot updates; remote serves tournaments after it.
func fixtureService(remote service.TournamentSource) *service.Service {
	return service.New(service.Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../../testdata/snapshot")) },
		Params: func() (rating.File, error) {
			return rating.File{Schema: rating.FileSchema, Params: rating.Params{K: 10, KNew: 24, NewBouts: 45, R: 0.3, WarmupScale: 0.5}, Show: true}, nil
		},
		Remote: remote,
		Clock:  clock.NewFake(testNow),
	})
}

// kyushu is a remote with one undecided bout on day 1 of Kyushu 2026.
type kyushu struct{}

var (
	kyushu2026 = domain.BashoID{Year: 2026, Month: time.November}
	y1         = domain.Rank{Title: domain.Yokozuna, Number: 1, Side: domain.East}
	o1         = domain.Rank{Title: domain.Ozeki, Number: 1, Side: domain.West}
)

func (kyushu) Basho(_ context.Context, id domain.BashoID) (domain.Basho, error) {
	start := domain.NewDate(2026, time.November, 8)
	return domain.Basho{ID: id, Start: start, End: start.AddDays(14)}, nil
}

func (kyushu) Banzuke(_ context.Context, _ domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	if div != domain.DivMakuuchi {
		return nil, nil
	}
	return []domain.BanzukeEntry{
		{RikishiID: 8850, Shikona: domain.Name{En: "Onosato", Jp: "大の里"}, Rank: y1},
		{RikishiID: 7, Shikona: domain.Name{En: "Kirishima", Jp: "霧島"}, Rank: o1},
	}, nil
}

func (kyushu) Torikumi(_ context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	return []domain.Bout{{
		Basho: id, Division: div, Day: day, MatchNo: 1,
		East: domain.BoutSide{RikishiID: 8850, Shikona: domain.Name{En: "Onosato"}, Rank: y1},
		West: domain.BoutSide{RikishiID: 7, Shikona: domain.Name{En: "Kirishima"}, Rank: o1},
	}}, nil
}

func openOn(t *testing.T, svc *service.Service, id domain.BashoID, day, match, width, height int) *tuitest.Driver {
	t.Helper()
	res, err := svc.Day(t.Context(), id, domain.DivMakuuchi, day)
	if err != nil {
		t.Fatal(err)
	}
	rows := append(res.Value.Bouts, res.Value.Playoffs...)
	for i, r := range rows {
		if r.MatchNo == match {
			return tuitest.New(tui.New(testContext(svc), []tui.Screen{New(id, domain.DivMakuuchi, day, res.Value.Date, rows, i)}, nil), width, height)
		}
	}
	t.Fatalf("%s day %d: no bout %d", id, day, match)
	return nil
}

// Golden snapshots of the VS screen (Phase 8 step 9).
func TestGolden(t *testing.T) {
	for _, tt := range []struct {
		name       string
		id         domain.BashoID
		day, match int
		width      int
	}{
		{"regular", aki2026, 13, 19, 100},
		{"regular_wide", aki2026, 13, 19, 140},
		{"regular_compact", aki2026, 13, 19, 70},
		{"kinboshi", domain.FirstBasho, 1, 21, 100},
		{"fusen", aki2026, 13, 15, 100},
		{"undecided", kyushu2026, 1, 1, 100},
		{"old_era", domain.FirstBasho, 15, 21, 100},
		{"newly_promoted", aki2026, 1, 2, 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openOn(t, fixtureService(kyushu{}), tt.id, tt.day, tt.match, tt.width, 40)
			golden.RequireEqual(t, d.View())
		})
	}
}

// Every Makuuchi bout of 202609 renders without layout problems at 90 and
// 130 columns (Phase 8 AC).
func TestAllBoutsRender(t *testing.T) {
	for day := 1; day <= domain.BashoDays; day++ {
		res, err := svc().Day(t.Context(), aki2026, domain.DivMakuuchi, day)
		if err != nil {
			t.Fatal(err)
		}
		for i, r := range res.Value.Bouts {
			for _, width := range []int{90, 130} {
				d := tuitest.New(tui.New(testContext(svc()), []tui.Screen{New(aki2026, domain.DivMakuuchi, day, res.Value.Date, res.Value.Bouts, i)}, nil), width, 40)
				v := d.View()
				name := fmt.Sprintf("day %d bout %d at %d", day, r.MatchNo, width)
				lines := strings.Split(v, "\n")
				if len(lines) != 40 {
					t.Errorf("%s: %d lines", name, len(lines))
				}
				for n, l := range lines {
					if w := displayWidth(l); w > width {
						t.Errorf("%s: line %d is %d wide", name, n, w)
					}
				}
				if !strings.Contains(v, "RECORD") || !strings.Contains(v, "wins by") || strings.Contains(strings.Split(lineWith(v, "HEYA"), "│")[0], "…") { // a placeholder; the wide history column is right of │
					t.Errorf("%s: incomplete:\n%s", name, v)
				}
			}
		}
	}
}

// forbidden fails the test if the snapshot is bypassed.
type forbidden struct{ t *testing.T }

var errForbidden = errors.New("a request")

func (f forbidden) Basho(context.Context, domain.BashoID) (domain.Basho, error) {
	f.t.Error("Basho requested")
	return domain.Basho{}, errForbidden
}

func (f forbidden) Banzuke(context.Context, domain.BashoID, domain.Division) ([]domain.BanzukeEntry, error) {
	f.t.Error("Banzuke requested")
	return nil, errForbidden
}

func (f forbidden) Torikumi(context.Context, domain.BashoID, domain.Division, int) ([]domain.Bout, error) {
	f.t.Error("Torikumi requested")
	return nil, errForbidden
}

// Opening a VS screen of a snapshot tournament makes no requests (Phase 8
// AC), also when stepping across days.
func TestNoRequests(t *testing.T) {
	d := openOn(t, fixtureService(forbidden{t}), aki2026, 13, 19, 100, 40)
	d.Keys("]", "[", "v", "m", "w")
	if !strings.Contains(d.View(), "MOVESET") {
		t.Error("the screen did not work")
	}
}
