package service

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// horizonAt is a fixed horizon for one tournament.
type horizonAt map[domain.BashoID]derive.AsOf

func (h horizonAt) Cutoff(id domain.BashoID) derive.AsOf {
	if c, ok := h[id]; ok {
		return c
	}
	return derive.BeforeDay(id, 1)
}

// The horizon after Makuuchi bout 5 of day 13.
var midDay13 = horizonAt{aki2026: {Basho: aki2026, Day: 13, Division: domain.DivMakuuchi, MatchNo: 5, Inclusive: true}}

func TestHidingDay(t *testing.T) {
	ctx := Hiding(t.Context(), midDay13)
	open, err := embedded().Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	if err != nil {
		t.Fatal(err)
	}
	hid, err := embedded().Day(ctx, aki2026, domain.DivMakuuchi, 13)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range hid.Value.Bouts {
		o := open.Value.Bouts[i]
		switch {
		case r.MatchNo <= 5 && (r.Hidden || r.Winner != o.Winner || r.Kimarite != o.Kimarite || r.East.After != o.East.After):
			t.Errorf("bout %d hidden: %+v", r.MatchNo, r)
		case r.MatchNo > 5 && (!r.Hidden || r.Winner != 0 || r.Kimarite.Kimarite != "" || r.Kinboshi || r.Fusen || r.East.After != r.East.Before):
			t.Errorf("bout %d shown: %+v", r.MatchNo, r)
		case r.East.Before != o.East.Before || r.West.Before != o.West.Before:
			t.Errorf("bout %d: records before differ on day 13", r.MatchNo)
		}
	}
	// Day 14 lies wholly after the horizon: records stay at the horizon.
	d14, err := embedded().Day(ctx, aki2026, domain.DivMakuuchi, 14)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range d14.Value.Bouts {
		if !r.Hidden {
			t.Errorf("day 14 bout %d shown", r.MatchNo)
		}
	}
	// Onosato fights in bout 19 of day 13: 11-1 before it, still 11-1 on day 14.
	for _, r := range d14.Value.Bouts {
		if r.East.RikishiID == 8850 || r.West.RikishiID == 8850 {
			rec := r.East.Before
			if r.West.RikishiID == 8850 {
				rec = r.West.Before
			}
			if rec.String() != "11-1" {
				t.Errorf("Onosato before day 14: %s", rec)
			}
		}
	}
	// Juryo day 15 hides its playoffs: that there is one gives the results away.
	j15, err := embedded().Day(ctx, aki2026, domain.DivJuryo, 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(j15.Value.Playoffs) != 0 {
		t.Error("playoffs shown")
	}
}

func TestHidingTale(t *testing.T) {
	ctx := Hiding(t.Context(), midDay13)
	for _, match := range []int{5, 19} {
		open, err := embedded().Bout(t.Context(), aki2026, domain.DivMakuuchi, 13, match)
		if err != nil {
			t.Fatal(err)
		}
		hid, err := embedded().Bout(ctx, aki2026, domain.DivMakuuchi, 13, match)
		if err != nil {
			t.Fatal(err)
		}
		if match == 5 && (hid.Value.Hidden || hid.Value.Result == nil) {
			t.Errorf("bout 5: hidden %v", hid.Value.Hidden)
		}
		if match == 19 && (!hid.Value.Hidden || hid.Value.Result != nil) {
			t.Errorf("bout 19: hidden %v, result %v", hid.Value.Hidden, hid.Value.Result)
		}
		// Bout 19 starts after the horizon: as of bout 5, ratings are
		// earlier than before bout 19; Onosato's record is the same (his
		// day 13 bout is 19 itself).
		if match == 19 {
			if hid.Value.East.Record != open.Value.East.Record {
				t.Errorf("record %s; before the bout %s", hid.Value.East.Record, open.Value.East.Record)
			}
		}
	}
	// A bout on day 14: its tale stops at the horizon.
	open, _ := embedded().Bout(t.Context(), aki2026, domain.DivMakuuchi, 14, 19)
	hid, _ := embedded().Bout(ctx, aki2026, domain.DivMakuuchi, 14, 19)
	if hid.Value.East.Record == open.Value.East.Record && hid.Value.West.Record == open.Value.West.Record {
		t.Error("day 14 tale shows day 13's results")
	}
	if a, b := hid.Value.East.Rating, open.Value.East.Rating; a != nil && b != nil && a.Value == b.Value {
		t.Error("the rating includes day 13's results after the horizon")
	}
	if len(hid.Value.HeadToHead.Meetings) > 0 && hid.Value.HeadToHead.Meetings[0].Basho == aki2026 && hid.Value.HeadToHead.Meetings[0].Day >= 13 {
		t.Error("head-to-head includes a later meeting")
	}
}

func TestHidingStandingsAndInfo(t *testing.T) {
	ctx := Hiding(t.Context(), midDay13)
	st, err := embedded().Standings(ctx, aki2026, domain.DivMakuuchi, 15)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Value.Limited || st.Value.Yusho != nil || st.Value.Today != 13 || st.Value.Rows[0].Record.String() != "11-1" {
		t.Errorf("standings: limited %v, yusho %v, today %d, top %s", st.Value.Limited, st.Value.Yusho, st.Value.Today, st.Value.Rows[0].Record)
	}
	info, err := embedded().Info(ctx, aki2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Value.Yusho) != 0 || len(info.Value.Prizes) != 0 {
		t.Errorf("info shows the winners: %+v", info.Value.Yusho)
	}
	// Past the end of the tournament everything shows.
	all := Hiding(t.Context(), horizonAt{aki2026: derive.EndOf(aki2026)})
	if info, _ := embedded().Info(all, aki2026); len(info.Value.Yusho) == 0 {
		t.Error("no yusho after the end")
	}
	if st, _ := embedded().Standings(all, aki2026, domain.DivMakuuchi, 15); st.Value.Limited || st.Value.Yusho == nil {
		t.Error("standings limited after the end")
	}
	// A tournament never revealed hides the yusho in the list too.
	list, err := embedded().List(ctx, 2026, 2026)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list.Value {
		if e.Yusho != nil {
			t.Errorf("%s: yusho shown", e.Basho)
		}
	}
}
