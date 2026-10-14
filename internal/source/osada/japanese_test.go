package osada

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestParseRank(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want domain.Rank
	}{
		{"横綱", domain.Rank{Title: domain.Yokozuna, Number: 1}},
		{"大関", domain.Rank{Title: domain.Ozeki, Number: 1}},
		{"関脇", domain.Rank{Title: domain.Sekiwake, Number: 1}},
		{"小結", domain.Rank{Title: domain.Komusubi, Number: 1}},
		{"前頭筆頭", domain.Rank{Title: domain.Maegashira, Number: 1}},
		{"前頭二枚目", domain.Rank{Title: domain.Maegashira, Number: 2}},
		{"前頭十枚目", domain.Rank{Title: domain.Maegashira, Number: 10}},
		{"前頭十七枚目", domain.Rank{Title: domain.Maegashira, Number: 17}},
		{"前頭1", domain.Rank{Title: domain.Maegashira, Number: 1}},
		{"十両14", domain.Rank{Title: domain.Juryo, Number: 14}},
		{"十両十一枚目", domain.Rank{Title: domain.Juryo, Number: 11}},
		{"幕下筆頭", domain.Rank{Title: domain.Makushita, Number: 1}},
		{"幕下五枚目", domain.Rank{Title: domain.Makushita, Number: 5}},
		{"張出横綱", domain.Rank{Title: domain.Yokozuna, Number: 1}},
		{"張出大関", domain.Rank{Title: domain.Ozeki, Number: 1}},
		{"前頭張出十枚目", domain.Rank{Title: domain.Maegashira, Number: 10}},
	} {
		got, err := parseRank(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("parseRank(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "三段目", "前頭十十", "前頭x", "十両零"} {
		if _, err := parseRank(bad); err == nil {
			t.Errorf("parseRank(%q): no error", bad)
		}
	}
}

func TestParseNumber(t *testing.T) {
	for in, want := range map[string]int{"一": 1, "九": 9, "十": 10, "十七": 17, "二十": 20, "二十三": 23, "百": 100, "百五": 105, "三百二十一": 321, "12": 12} {
		if got, err := parseNumber(in); err != nil || got != want {
			t.Errorf("parseNumber(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}

func TestParseBasho(t *testing.T) {
	for _, tt := range []struct{ year, name, want string }{
		{"令和八年", "九月場所", "202609"},
		{"令和八年", "十一月場所", "202611"},
		{"令和元年", "五月場所", "201905"},
		{"令和十年", "一月場所", "202801"},
	} {
		got, err := parseBasho(tt.year, tt.name)
		if err != nil || got.String() != tt.want {
			t.Errorf("parseBasho(%q, %q) = %v, %v; want %s", tt.year, tt.name, got, err, tt.want)
		}
	}
	for _, bad := range [][2]string{{"平成三十年", "九月場所"}, {"令和八年", "二月場所"}, {"令和八年", "秋場所"}} {
		if _, err := parseBasho(bad[0], bad[1]); err == nil {
			t.Errorf("parseBasho(%q, %q): no error", bad[0], bad[1])
		}
	}
}

func TestKimarite(t *testing.T) {
	for in, want := range map[string]domain.Kimarite{
		"寄り切り": "yorikiri", "押し出し": "oshidashi", "上手出し投げ": "uwatedashinage", "とったり": "tottari",
		"勇み足": "isamiashi", "浴せ倒し": "abisetaoshi", "不戦": "fusen", "未定": "", "": "", "謎の技": "謎の技",
	} {
		if got := kimarite(in); got != want {
			t.Errorf("kimarite(%q) = %q; want %q", in, got, want)
		}
	}
}

// Every technique in the recorded tournament is in the kimarite table.
func TestFixtureKimariteKnown(t *testing.T) {
	cur := fixtureCurrent(t)
	for _, d := range cur.Days {
		for div, bouts := range d.Bouts {
			for _, b := range bouts {
				if _, ok := b.Kimarite.Info(); b.Kimarite != "" && !ok {
					t.Errorf("day %d %s bout %d: unknown %q", d.Day, div, b.MatchNo, b.Kimarite)
				}
			}
		}
	}
}
