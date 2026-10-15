package tui_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

func namesContext(mode domain.NameMode) *tui.Context {
	return &tui.Context{Svc: embedded(), Cfg: config.Default(), Theme: theme.New(true, false).WithNames(mode), Clock: clock.NewFake(testNow), Tick: tui.NoTimers}
}

func isKanji(r rune) bool { return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) }

// Phase 13 AC: with ui.names = en no screen shows kanji, kana or the 結び
// and 休 marks; with jp the kanji names lead and every screen still fits.
func TestNameModes(t *testing.T) {
	if testing.Short() || raceDetector {
		t.Skip("walks every screen; make check runs it")
	}
	for name, view := range screens(t, namesContext(domain.NamesEn), []int{1, 13, 15}) {
		if i := strings.IndexFunc(view, isKanji); i >= 0 {
			t.Errorf("en: %s shows %q:\n%s", name, []rune(view[i:])[0], view)
		}
	}
	shown := screens(t, namesContext(domain.NamesJp), []int{13})
	for name, view := range shown {
		for i, l := range strings.Split(view, "\n") {
			if w := ansi.StringWidth(l); w > 140 {
				t.Errorf("jp: %s line %d is %d wide", name, i, w)
			}
		}
	}
	if v := shown["makuuchi day 13"]; !strings.Contains(v, "大の里") || strings.Contains(v, "Onosato") {
		t.Errorf("jp day view:\n%s", v)
	}
	if v := shown["makuuchi day 13 VS 19"]; !strings.Contains(v, "大の里") || !strings.Contains(v, "Onosato") {
		t.Errorf("jp VS screen leads with kanji and keeps romaji beside:\n%s", v)
	}
}

func TestNameGoldens(t *testing.T) {
	for _, mode := range []struct {
		name string
		mode domain.NameMode
	}{{"en", domain.NamesEn}, {"jp", domain.NamesJp}} {
		for _, size := range []struct{ w, h int }{{80, 24}, {120, 40}} {
			ctx := namesContext(mode.mode)
			ctx.Svc = fixtureService()
			d := tuitest.New(appWith(ctx), size.w, size.h)
			d.Keys("g", "1", "3")
			t.Run(fmt.Sprintf("%s_day13_%dx%d", mode.name, size.w, size.h), func(t *testing.T) { golden.RequireEqual(t, d.View()) })
			d.Keys(slices.Repeat([]string{"j"}, 18)...)
			d.Keys("enter")
			t.Run(fmt.Sprintf("%s_musubi_%dx%d", mode.name, size.w, size.h), func(t *testing.T) { golden.RequireEqual(t, d.View()) })
		}
	}
}
