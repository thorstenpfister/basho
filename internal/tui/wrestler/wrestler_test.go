package wrestler

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

var (
	testNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)
	aki2026 = domain.BashoID{Year: 2026, Month: time.September}
)

const onosato = 8850

// fixture is the fixture snapshot (196001, 202607, 202609) with a generated
// portrait for Onosato: stable across data runs.
func fixture(t *testing.T) *service.Service {
	t.Helper()
	files := fstest.MapFS{"credits.json": {Data: []byte(fmt.Sprintf(`[{"id":%d,"name":"Onosato","file":"x.jpg","author":"A","licence":"CC0","sourceUrl":"u","year":2024}]`, onosato))}}
	for _, size := range portrait.Sizes {
		img := image.NewPaletted(image.Rect(0, 0, size, size), color.Palette{color.NRGBA{200, 140, 110, 255}, color.NRGBA{40, 30, 60, 255}})
		for y := range size {
			for x := range size {
				if x < y {
					img.SetColorIndex(x, y, 1)
				}
			}
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		files[portrait.Path(onosato, size)] = &fstest.MapFile{Data: b.Bytes()}
	}
	return service.New(service.Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../../testdata/snapshot")) },
		Params: func() (rating.File, error) {
			return rating.File{Schema: rating.FileSchema, Params: rating.Params{K: 10, KNew: 24, NewBouts: 45, R: 0.3, WarmupScale: 0.5}, Show: true}, nil
		},
		Portraits: func() (*portrait.Set, error) { return portrait.Open(files) },
		Clock:     clock.NewFake(testNow),
	})
}

type opened struct {
	id  domain.BashoID
	div domain.Division
	day int
}

// stub is a screen that only says what was opened.
type stub struct{ opened }

func (s stub) Init(*tui.Context) tea.Cmd                          { return nil }
func (s stub) Update(*tui.Context, tea.Msg) (tui.Screen, tea.Cmd) { return s, nil }
func (s stub) View(*tui.Context, int, int) string {
	return fmt.Sprintf("day view %s %s %d", s.id, s.div, s.day)
}
func (s stub) Status(*tui.Context) tui.Status { return tui.Status{} }
func (s stub) Keys() []key.Binding            { return nil }

func open(t *testing.T, id domain.RikishiID, width, height int) (*tuitest.Driver, *tui.Context) {
	t.Helper()
	ctx := &tui.Context{Svc: fixture(t), Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers}
	ctx.OpenDay = func(id domain.BashoID, div domain.Division, day int) tui.Screen { return stub{opened{id, div, day}} }
	return tuitest.New(tui.New(ctx, []tui.Screen{New(id, aki2026)}, nil), width, height), ctx
}

func TestGolden(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {120, 40}, {160, 50}} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			d, _ := open(t, onosato, size.w, size.h)
			v := d.View()
			for i, l := range strings.Split(v, "\n") {
				if w := ansi.StringWidth(l); w > size.w {
					t.Errorf("line %d is %d wide", i, w)
				}
			}
			if lines := strings.Count(v, "\n") + 1; lines != size.h {
				t.Errorf("%d lines for %d rows", lines, size.h)
			}
			golden.RequireEqual(t, v)
		})
	}
}

func TestNavigation(t *testing.T) {
	d, _ := open(t, onosato, 120, 40)
	if v := d.View(); !strings.Contains(v, "▶ Aki    2026") || !strings.Contains(v, "photo") && !strings.Contains(v, "ONOSATO") {
		t.Fatalf("newest first, cursor on it:\n%s", v)
	}
	d.Keys("j")
	if v := d.View(); !strings.Contains(v, "▶ Nagoya 2026") {
		t.Errorf("j: \n%s", v)
	}
	d.Keys("G", "g", "j", "enter")
	if v := d.View(); !strings.Contains(v, "day view 202607 Makuuchi 15") {
		t.Errorf("enter opens Nagoya 2026 on his last bout's day:\n%s", v)
	}
}

// ui.portraits = false (or --no-portraits) hides the portrait; the facts
// stay.
func TestNoPortrait(t *testing.T) {
	d, ctx := open(t, onosato, 160, 50)
	if v := d.View(); !strings.Contains(v, "▀") {
		t.Fatal("no portrait to begin with")
	}
	ctx.Cfg.UI.Portraits = false
	d = tuitest.New(tui.New(ctx, []tui.Screen{New(onosato, aki2026)}, nil), 160, 50)
	if v := d.View(); strings.ContainsAny(v, "▀▄") || !strings.Contains(v, "RANK       Yokozuna 1 East") {
		t.Errorf("with ui.portraits = false:\n%s", v)
	}
}
