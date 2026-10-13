package domain

import (
	"slices"
	"testing"
)

func TestParseRank(t *testing.T) {
	tests := []struct {
		in    string
		want  Rank
		short string
	}{
		{"Yokozuna 1 East", Rank{Yokozuna, 1, East}, "Y1e"},
		{"Ozeki 2 West", Rank{Ozeki, 2, West}, "O2w"},
		{"Maegashira 17 East", Rank{Maegashira, 17, East}, "M17e"},
		{"Juryo 2 West", Rank{Juryo, 2, West}, "J2w"},
		{"Makushita 3 East", Rank{Makushita, 3, East}, "Ms3e"},
	}
	for _, tt := range tests {
		got, err := ParseRank(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseRank(%q) = %+v, %v; want %+v", tt.in, got, err, tt.want)
			continue
		}
		if got.String() != tt.in || got.Short() != tt.short {
			t.Errorf("%q: String %q, Short %q; want %q, %q", tt.in, got.String(), got.Short(), tt.in, tt.short)
		}
	}
	if (Rank{}).String() != "" || (Rank{}).Short() != "" {
		t.Error("the zero Rank must print as empty")
	}
	if got := (Rank{Title: Yokozuna, Number: 1}).Short(); got != "Y1" {
		t.Errorf("Short without a side = %q, want Y1", got)
	}
	for _, bad := range []string{"", "Yokozuna", "Yokozuna 1", "Shogun 1 East", "Maegashira 0 East", "Maegashira x East", "Maegashira 1 North", "Maegashira 1 East extra"} {
		if r, err := ParseRank(bad); err == nil {
			t.Errorf("ParseRank(%q) = %+v, want an error", bad, r)
		}
	}
}

func TestRankOrder(t *testing.T) {
	ordered := []Rank{
		{Yokozuna, 1, East},
		{Yokozuna, 1, West},
		{Ozeki, 1, East},
		{Sekiwake, 1, West},
		{Komusubi, 2, East},
		{Maegashira, 1, East},
		{Maegashira, 17, West},
		{Juryo, 1, East},
		{Juryo, 14, West},
		{Makushita, 1, East},
	}
	shuffled := slices.Clone(ordered)
	slices.Reverse(shuffled)
	slices.SortFunc(shuffled, Rank.Compare)
	if !slices.Equal(shuffled, ordered) {
		t.Errorf("sorted = %v", shuffled)
	}
}

func TestRankTitleProperties(t *testing.T) {
	for title, want := range map[RankTitle]struct {
		sanyaku bool
		div     Division
	}{
		Yokozuna:   {true, DivMakuuchi},
		Komusubi:   {true, DivMakuuchi},
		Maegashira: {false, DivMakuuchi},
		Juryo:      {false, DivJuryo},
		Makushita:  {false, DivMakushita},
	} {
		if title.Sanyaku() != want.sanyaku || title.Division() != want.div {
			t.Errorf("%s: Sanyaku %v, Division %s; want %v, %s", title, title.Sanyaku(), title.Division(), want.sanyaku, want.div)
		}
	}
}
