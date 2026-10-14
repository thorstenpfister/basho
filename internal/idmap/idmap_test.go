package idmap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestMap(t *testing.T) {
	m := New([]domain.Rikishi{{ID: 8850, NSKID: 4227}, {ID: 3081}}) // Onosato; Hakuho without a JSA ID here
	if id, ok := m.ByJSA(4227); !ok || id != 8850 {
		t.Errorf("4227 → %d, %v", id, ok)
	}
	if _, ok := m.ByJSA(0); ok || m.Len() != 1 {
		t.Error("an entry without a JSA ID was mapped")
	}
	m.Add([]domain.Rikishi{{ID: 9000, NSKID: 4227}})
	if id, _ := m.ByJSA(4227); id != 9000 {
		t.Errorf("update: %d", id)
	}
}

func TestRomaji(t *testing.T) {
	for in, want := range map[string]string{
		"おおのさと":     "Onosato",
		"ほうしょうりゅう":  "Hoshoryu",
		"ことしょうほう":   "Kotoshoho",
		"きりしま":      "Kirishima",
		"あおにしき":     "Aonishiki",
		"たかやす":      "Takayasu",
		"ちよしょうま":    "Chiyoshoma",
		"わかもとはる":    "Wakamotoharu",
		"うら":        "Ura",
		"しし":        "Shishi",
		"あたみふじ":     "Atamifuji",
		"いちやまもと":    "Ichiyamamoto",
		"きんぼうざん":    "Kinbozan",
		"じょうみゃくりゅう": "Jomyakuryu",
		"ほっかいふじ":    "Hokkaifuji",
		"はっちょう":     "Hatcho",
		"オオノサト":     "Onosato",
		"":          "",
	} {
		if got := Romaji(in); got != want {
			t.Errorf("Romaji(%q) = %q; want %q", in, got, want)
		}
	}
	// Known to be imperfect (SPEC §5.6): 海 after の or ど is collapsed like
	// a long vowel.
	for in, want := range map[string]string{"しょうなんのうみ": "Shonannomi", "ひらどうみ": "Hiradomi"} {
		if got := Romaji(in); got != want {
			t.Errorf("Romaji(%q) = %q; want %q", in, got, want)
		}
	}
}

type fetcher struct {
	calls int
	rs    []domain.Rikishi
}

func (f *fetcher) fetch(context.Context) ([]domain.Rikishi, error) {
	f.calls++
	return f.rs, nil
}

func TestResolve(t *testing.T) {
	onosato := domain.Rikishi{ID: 8850, NSKID: 4227, Name: domain.Name{En: "Onosato", Jp: "大の里"}}
	m := New([]domain.Rikishi{onosato})
	now := time.Date(2026, time.November, 8, 15, 0, 0, 0, domain.JST)
	f := &fetcher{}
	m.SetRefresh(f.fetch, func() time.Time { return now })

	if got := m.Resolve(t.Context(), 4227, "", "おおのさと"); got.ID != 8850 || got.Temporary || got.Name != (domain.Name{En: "Onosato", Jp: "大の里", Kana: "おおのさと"}) {
		t.Errorf("known: %+v", got)
	}
	if f.calls != 0 {
		t.Error("a known ID refreshed")
	}

	// A newcomer: one refresh, then a temporary identity.
	got := m.Resolve(t.Context(), 4400, "新人", "しんじん")
	if !got.Temporary || got.ID != TemporaryID(4400) || got.Name.Romaji() != "≈Shinjin" || got.Name.Jp != "新人" || f.calls != 1 {
		t.Errorf("newcomer: %+v after %d refreshes", got, f.calls)
	}
	m.Resolve(t.Context(), 4400, "新人", "しんじん")
	m.Resolve(t.Context(), 4401, "", "べつじん")
	if f.calls != 1 {
		t.Errorf("%d refreshes within the hour", f.calls)
	}

	// An hour later the index has them.
	now = now.Add(RefreshInterval)
	f.rs = []domain.Rikishi{{ID: 9100, NSKID: 4400, Name: domain.Name{En: "Shinjin"}}}
	if got := m.Resolve(t.Context(), 4400, "新人", "しんじん"); got.Temporary || got.ID != 9100 || got.Name.Approx || f.calls != 2 {
		t.Errorf("after the refresh: %+v, %d refreshes", got, f.calls)
	}

	// Without a refresh function, unknown IDs are temporary at once.
	if got := New(nil).Resolve(t.Context(), 1, "", "あ"); !got.Temporary {
		t.Error("no refresh: not temporary")
	}
}

func TestRefreshError(t *testing.T) {
	m := New(nil)
	calls := 0
	m.SetRefresh(func(context.Context) ([]domain.Rikishi, error) { calls++; return nil, errors.New("down") }, time.Now)
	m.Resolve(t.Context(), 1, "", "あ")
	m.Resolve(t.Context(), 2, "", "い")
	if calls != 1 {
		t.Errorf("%d refreshes: a failure counts towards the hour too", calls)
	}
}
