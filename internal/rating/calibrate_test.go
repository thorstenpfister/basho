package rating

import (
	"math"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestScorer(t *testing.T) {
	var s scorer
	s.add(0.8, true)  // right
	s.add(0.8, false) // wrong
	s.add(0.5, true)  // half
	s.add(0.25, false)
	m := s.metrics()
	wantLL := -(math.Log(0.8) + math.Log(0.2) + math.Log(0.5) + math.Log(0.75)) / 4
	wantBrier := (0.04 + 0.64 + 0.25 + 0.0625) / 4
	if m.Bouts != 4 || !near(m.LogLoss, wantLL) || !near(m.Brier, wantBrier) || !near(m.Accuracy, 2.5/4) {
		t.Errorf("metrics %+v", m)
	}
	if b := m.Calibration[8]; b.Bouts != 2 || !near(b.Predicted, 0.8) || b.Actual != 0.5 {
		t.Errorf("80%% bucket %+v", b)
	}
	if b := m.Calibration[5]; b.Bouts != 1 {
		t.Errorf("50%% falls in the 50–60%% bucket: %+v", b)
	}
	var edge scorer
	edge.add(1, false) // clamped, not infinite
	if m := edge.metrics(); math.IsInf(m.LogLoss, 0) || m.Calibration[9].Bouts != 1 {
		t.Errorf("certainty %+v", m)
	}
	if empty := (&scorer{}).metrics(); empty != (Metrics{}) {
		t.Error("empty")
	}
}

func TestGoldenMin(t *testing.T) {
	if x := goldenMin(0, 1, func(x float64) float64 { return (x - 0.3) * (x - 0.3) }); math.Abs(x-0.3) > 1e-5 {
		t.Errorf("minimum at %.6f", x)
	}
}

// history makes tournaments from 1960 in which wrestler 1 (Yokozuna) beats
// wrestler 2 (Maegashira) on every day, and 3 beats 4 on even days.
func history(n int) [][]domain.Bout {
	y1, m1 := rank(domain.Yokozuna, 1), rank(domain.Maegashira, 1)
	var out [][]domain.Bout
	id := domain.FirstBasho
	for range n {
		var bouts []domain.Bout
		for day := 1; day <= domain.BashoDays; day++ {
			bouts = append(bouts, bout(id, day, 1, 2, y1, m1, true), bout(id, day, 3, 4, m1, m1, day%2 == 0))
			bouts[len(bouts)-1].MatchNo = 2
		}
		out = append(out, bouts)
		id = id.Next()
	}
	return out
}

func TestEvaluateAndCalibrate(t *testing.T) {
	ts := history(12) // 196001–196111
	s := Split{TrainFrom: domain.BashoID{Year: 1960, Month: time.May}, EvalFrom: domain.BashoID{Year: 1961, Month: time.January}, Through: domain.BashoID{Year: 1961, Month: time.September}}
	r := Evaluate(params, ts, s)
	if r.Train.Bouts != 4*30 || r.Eval.Bouts != 5*30 {
		t.Errorf("bouts %d train, %d eval; want 120 and 150 (through 196109)", r.Train.Bouts, r.Eval.Bouts)
	}
	if again := Evaluate(params, ts, s); again != r {
		t.Error("not deterministic")
	}

	grid := []Params{
		{K: 32, KNew: 32, NewBouts: 1, WarmupScale: 1},
		{K: 4, KNew: 4, NewBouts: 1, WarmupScale: 1},
		{K: 16, KNew: 16, NewBouts: 1, WarmupScale: 1},
	}
	rs := Calibrate(grid, ts, s)
	for i := 1; i < len(rs); i++ {
		if rs[i-1].Train.LogLoss > rs[i].Train.LogLoss {
			t.Errorf("not sorted by training log loss: %v", rs)
		}
	}
	// The Yokozuna always wins: faster learning is better here.
	if rs[0].Params.K != 32 {
		t.Errorf("chosen K %g", rs[0].Params.K)
	}
	// Ties go to the smaller K: here every bout counts as new, so K never
	// applies.
	same := Calibrate([]Params{{K: 20, KNew: 16, NewBouts: 1000}, {K: 10, KNew: 16, NewBouts: 1000}}, ts, s)
	if same[0].Params.K != 10 {
		t.Errorf("tie went to K %g", same[0].Params.K)
	}

	b := FitBaselines(ts, s)
	if b.RankSlope <= 0 || b.Rank.Eval.LogLoss >= b.Coin.Eval.LogLoss || !near(b.Coin.Eval.LogLoss, math.Ln2) {
		t.Errorf("baselines %+v", b)
	}
}

func TestUpTo(t *testing.T) {
	ts := history(3)
	if got := upTo(ts, domain.BashoID{Year: 1960, Month: time.March}); len(got) != 2 {
		t.Errorf("through 196003: %d tournaments", len(got))
	}
	if got := upTo(ts, domain.BashoID{Year: 2026, Month: time.September}); len(got) != 3 {
		t.Errorf("through later: %d", len(got))
	}
	if got := upTo(ts, domain.BashoID{Year: 1959, Month: time.November}); len(got) != 0 {
		t.Errorf("through earlier: %d", len(got))
	}
}

func TestGrid(t *testing.T) {
	if n := len(Grid()); n != 7*4*3*5*3 {
		t.Errorf("%d parameter sets", n)
	}
}
