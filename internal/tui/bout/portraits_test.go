package bout

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

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

// withPortrait is the fixture service with a portrait for the musubi's
// East wrestler only: a gradient, 2024 (no photo goes into the repository).
func withPortrait(t *testing.T) *service.Service {
	t.Helper()
	east := onosato(t)
	files := fstest.MapFS{"credits.json": {Data: []byte(fmt.Sprintf(`[{"id":%d,"name":"Onosato","file":"x.jpg","author":"A","licence":"CC0","sourceUrl":"u","year":2024}]`, east))}}
	for _, size := range portrait.Sizes {
		img := image.NewPaletted(image.Rect(0, 0, size, size), color.Palette{color.NRGBA{200, 140, 110, 255}, color.NRGBA{40, 30, 60, 255}})
		for y := range size {
			for x := range size {
				if x+y < size {
					img.SetColorIndex(x, y, 1)
				}
			}
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		files[portrait.Path(east, size)] = &fstest.MapFile{Data: b.Bytes()}
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

func onosato(t *testing.T) domain.RikishiID {
	t.Helper()
	res, err := fixtureService(nil).Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	if err != nil {
		t.Fatal(err)
	}
	return res.Value.Bouts[len(res.Value.Bouts)-1].East.RikishiID
}

// Portraits beside the tape at the sizes the width allows (SPEC §8.9):
// the East photo facing right, a silhouette for West without one.
func TestPortraitGolden(t *testing.T) {
	for _, size := range []struct{ w, h int }{{200, 50}, {150, 50}, {100, 42}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			d := openOn(t, withPortrait(t), aki2026, 13, 19, size.w, size.h)
			v := d.View()
			for i, l := range strings.Split(v, "\n") {
				if w := ansi.StringWidth(l); w > size.w {
					t.Errorf("line %d is %d wide", i, w)
				}
			}
			if size.h >= 42 && !strings.Contains(v, "photo 2024") {
				t.Errorf("no caption:\n%s", v)
			}
			golden.RequireEqual(t, v)
		})
	}
}

func TestPortraitSize(t *testing.T) {
	svc := withPortrait(t)
	ctx := &tui.Context{Svc: svc, Cfg: testContext(nil).Cfg, Theme: theme.New(true, false), Clock: clock.NewFake(testNow)}
	res, err := svc.Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Value.Bouts
	musubi := New(aki2026, domain.DivMakuuchi, 13, res.Value.Date, rows, len(rows)-1) // Onosato has a photo
	other := New(aki2026, domain.DivMakuuchi, 13, res.Value.Date, rows, 0)            // nobody has
	musubi.tale, other.tale = &service.Tale{}, &service.Tale{}                        // loaded
	// 32 px needs 16 rows and the caption. Every column outside the compact
	// layout is at least 37 wide, so the height decides.
	for _, tt := range []struct{ width, rows, want int }{
		{120, 17, 32}, {120, 16, 20}, {120, 11, 20}, {120, 10, 0}, {200, 40, 32}, {90, 40, 32}, {80, 40, 0}, // compact: never
	} {
		if got := musubi.portraitSize(ctx, measure(tt.width), tt.rows); got != tt.want {
			t.Errorf("%d columns, %d rows: %d px, want %d", tt.width, tt.rows, got, tt.want)
		}
	}
	loading := musubi
	loading.tale = nil
	if loading.portraitSize(ctx, measure(120), 40) != 0 {
		t.Error("a portrait before the tale is in")
	}
	if other.portraitSize(ctx, measure(120), 40) != 0 {
		t.Error("two silhouettes shown")
	}
	for name, change := range map[string]func(*tui.Context){
		"ui.portraits = false": func(c *tui.Context) { c.Cfg.UI.Portraits = false },
		"--ascii":              func(c *tui.Context) { c.Theme = theme.New(true, true) },
		"NO_COLOR":             func(c *tui.Context) { c.Theme.Mono = true },
	} {
		c := *ctx
		change(&c)
		if musubi.portraitSize(&c, measure(120), 40) != 0 {
			t.Errorf("%s shows portraits", name)
		}
	}
}
