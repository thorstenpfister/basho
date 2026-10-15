package components

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

var (
	uni   = theme.New(true, false)
	ascii = theme.New(true, true)
)

func plain(s string) string { return ansi.Strip(s) }

func TestStatRow(t *testing.T) {
	got := plain(StatRow(uni, "11-1", "RECORD", "6-6", 10, 12))
	if got != "      11-1   RECORD   6-6       " {
		t.Errorf("%q", got)
	}
	if w := ansi.StringWidth(StatRow(uni, "Ishikawa-ken, Kahoku-gun", "FROM", "大の里大の里大の里", 10, 6)); w != 26 {
		t.Errorf("clipped row is %d wide", w)
	}
}

func TestBar(t *testing.T) {
	if got := plain(Bar(uni, 190, 170, 190, 10, true)); got != "██████████" {
		t.Errorf("max %q", got)
	}
	if got := plain(Bar(uni, 170, 170, 190, 10, false)); got != "██        " {
		t.Errorf("min %q", got)
	}
	if got := plain(Bar(uni, 180, 170, 190, 10, true)); got != "    ██████" {
		t.Errorf("middle, left %q", got)
	}
	if Bar(uni, 0, 170, 190, 10, true) != "" {
		t.Error("an unknown value draws nothing")
	}
	if got := plain(Bar(ascii, 180, 180, 180, 4, false)); got != "####" {
		t.Errorf("one value, ascii %q", got)
	}
}

func TestPips(t *testing.T) {
	nago, aki := domain.BashoID{Year: 2026, Month: time.July}, domain.BashoID{Year: 2026, Month: time.September}
	l := derive.LastN{Results: []derive.Result{
		{Basho: nago, Won: true}, {Basho: nago, Won: false, Fusen: true}, {Basho: aki, Won: true, Fusen: true},
	}, New: true}
	if got := plain(Pips(uni, l, 5)); got != "○■│□··" {
		t.Errorf("%q", got)
	}
	if got := plain(Pips(ascii, l, 5)); got != "oX|O.." {
		t.Errorf("ascii %q", got)
	}
}

func TestStars(t *testing.T) {
	for _, tt := range []struct {
		k    derive.Kinboshi
		want string
	}{
		{derive.Kinboshi{}, "–"},
		{derive.Kinboshi{Count: 2, ThisBasho: 1}, "★★"},
		{derive.Kinboshi{Count: 12}, "★×12"},
	} {
		if got := plain(Stars(uni, tt.k)); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.k, got, tt.want)
		}
	}
	if Stars(uni, derive.Kinboshi{Count: 2, ThisBasho: 1}) == Stars(uni, derive.Kinboshi{Count: 2}) {
		t.Error("this tournament's stars are not brighter")
	}
}

func TestWinChance(t *testing.T) {
	l, r := WinChance(uni, 0.58, 20)
	if plain(l) != strings.Repeat("━", 12)+" 58%" || plain(r) != "42% "+strings.Repeat("━", 8) {
		t.Errorf("%q %q", plain(l), plain(r))
	}
}

func TestStyleBar(t *testing.T) {
	var s derive.Shares
	s[domain.Push], s[domain.Belt], s[domain.Throw], s[domain.Pull], s[domain.Other] = 0.58, 0.21, 0.10, 0.10, 0.01
	right := plain(StyleBar(ascii, s, StyleCells, false))
	if right != "PPPPPPPPPPPPBBBBTTLL" {
		t.Errorf("right %q", right)
	}
	if left := plain(StyleBar(ascii, s, StyleCells, true)); left != "LLTTBBBBPPPPPPPPPPPP" {
		t.Errorf("left %q", left)
	}
	if got := ansi.StringWidth(StyleBar(uni, s, StyleCells, true)); got != StyleCells {
		t.Errorf("width %d", got)
	}
	if StyleBar(uni, derive.Shares{}, StyleCells, false) != "" {
		t.Error("no wins, no bar")
	}
	mono := uni
	mono.Mono = true // without colours, letters tell the categories apart
	if got := plain(StyleBar(mono, s, StyleCells, false)); got != "PPPPPPPPPPPPBBBBTTLL" {
		t.Errorf("mono %q", got)
	}
}

func TestApportion(t *testing.T) {
	var s derive.Shares
	s[domain.Push], s[domain.Belt], s[domain.Throw] = 1.0/3, 1.0/3, 1.0/3
	n := apportion(s, 20)
	if n[domain.Push]+n[domain.Belt]+n[domain.Throw] != 20 {
		t.Errorf("%v does not add up", n)
	}
}
