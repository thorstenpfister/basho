package rating_test

import (
	"slices"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
)

// embedded returns the embedded snapshot's tournaments in order of play.
func embedded(t *testing.T) [][]domain.Bout {
	t.Helper()
	s, err := data.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	x, err := s.BuildIndex()
	if err != nil {
		t.Fatal(err)
	}
	var ts [][]domain.Bout
	for _, id := range x.Tournaments() {
		bouts := slices.Clone(x.Tournament(id))
		slices.SortStableFunc(bouts, derive.ComparePlay)
		ts = append(ts, bouts)
	}
	return ts
}

// The committed params.json reproduces its evaluation scores exactly: the
// model is deterministic, and a change to it has to show in this test that
// it does not get worse (SPEC §13).
func TestCommittedCalibration(t *testing.T) {
	f, err := data.RatingParams()
	if err != nil {
		t.Fatal(err)
	}
	ts := embedded(t)
	got := rating.Evaluate(f.Params, ts, f.Split)
	if got.Eval != f.Eval {
		t.Errorf("evaluation\n%+v\nparams.json\n%+v", got.Eval, f.Eval)
	}
	b := rating.FitBaselines(ts, f.Split)
	if b.Rank.Eval != f.RankEval || b.RankSlope != f.RankSlope {
		t.Errorf("rank baseline %+v (slope %v), params.json %+v (%v)", b.Rank.Eval, b.RankSlope, f.RankEval, f.RankSlope)
	}
	if f.Show != (f.Eval.LogLoss < f.RankEval.LogLoss) {
		t.Errorf("show %v does not follow from the scores", f.Show)
	}
}

// Win chances for a sample of real bouts are plausible.
func TestSampleWinChances(t *testing.T) {
	f, err := data.RatingParams()
	if err != nil {
		t.Fatal(err)
	}
	r := rating.New(f.Params)
	for _, bouts := range embedded(t) {
		if err := r.Set(bouts[0].Basho, bouts); err != nil {
			t.Fatal(err)
		}
	}
	hatsu2011 := domain.BashoID{Year: 2011, Month: time.January}
	y1 := domain.Rank{Title: domain.Yokozuna, Number: 1, Side: domain.East}
	m10 := domain.Rank{Title: domain.Maegashira, Number: 10, Side: domain.West}
	c := derive.BeforeDay(hatsu2011, 1)
	hakuho, _ := r.Before(3081, y1, c)
	debut, prov := r.Before(999999, m10, c) // nobody: a debut at M10
	if !prov {
		t.Error("an unknown wrestler is not provisional")
	}
	if p := rating.WinChance(hakuho, debut); p < 0.85 || p > 0.99 {
		t.Errorf("Hakuho at his peak against a new M10: %.2f", p)
	}
	// Two runs give the same numbers.
	again := rating.New(f.Params)
	for _, bouts := range embedded(t) {
		_ = again.Set(bouts[0].Basho, bouts)
	}
	if h2, _ := again.Before(3081, y1, c); h2 != hakuho {
		t.Errorf("not deterministic: %v and %v", hakuho, h2)
	}
}
