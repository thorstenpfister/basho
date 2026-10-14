package rating

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// Split divides the history for calibration (SPEC §6.8): the parameters are
// chosen on the training years and judged on the evaluation years.
type Split struct {
	TrainFrom domain.BashoID `json:"trainFrom"` // after the warm-up
	EvalFrom  domain.BashoID `json:"evalFrom"`
	Through   domain.BashoID `json:"through"` // the newest tournament used
}

// DefaultSplit trains on 1965–2014 and evaluates from 2015.
func DefaultSplit(through domain.BashoID) Split {
	return Split{TrainFrom: domain.BashoID{Year: 1965, Month: 1}, EvalFrom: domain.BashoID{Year: 2015, Month: 1}, Through: through}
}

// Metrics scores win chances against results.
type Metrics struct {
	Bouts       int        `json:"bouts"`
	LogLoss     float64    `json:"logLoss"`
	Brier       float64    `json:"brier"`
	Accuracy    float64    `json:"accuracy"`
	Calibration [10]Bucket `json:"calibration"` // by predicted chance of the east side, 10% wide
}

// Bucket is one row of the calibration table.
type Bucket struct {
	Bouts     int     `json:"bouts"`
	Predicted float64 `json:"predicted"` // mean predicted chance
	Actual    float64 `json:"actual"`    // share won
}

type scorer struct {
	n, correct2 int // correct2 counts halves: a 50% prediction is half right
	ll, brier   float64
	buckets     [10]struct {
		n      int
		p, won float64
	}
}

func (s *scorer) add(p float64, won bool) {
	y := 0.0
	if won {
		y = 1
	}
	q := min(max(p, 1e-15), 1-1e-15)
	s.n++
	s.ll -= y*math.Log(q) + (1-y)*math.Log(1-q)
	s.brier += (p - y) * (p - y)
	switch {
	case p == 0.5:
		s.correct2++
	case (p > 0.5) == won:
		s.correct2 += 2
	}
	b := &s.buckets[min(int(p*10), 9)]
	b.n++
	b.p += p
	b.won += y
}

func (s *scorer) metrics() Metrics {
	m := Metrics{Bouts: s.n}
	if s.n == 0 {
		return m
	}
	n := float64(s.n)
	m.LogLoss, m.Brier, m.Accuracy = s.ll/n, s.brier/n, float64(s.correct2)/2/n
	for i, b := range s.buckets {
		if b.n > 0 {
			m.Calibration[i] = Bucket{Bouts: b.n, Predicted: b.p / float64(b.n), Actual: b.won / float64(b.n)}
		}
	}
	return m
}

// Result is one parameter set's scores.
type Result struct {
	Params      Params  `json:"params"`
	Train, Eval Metrics `json:"-"`
}

// Evaluate runs the model over tournaments (oldest first, each in any order)
// and scores it on both parts of the split.
func Evaluate(p Params, tournaments [][]domain.Bout, s Split) Result {
	var train, eval scorer
	Replay(p, upTo(tournaments, s.Through), func(b domain.Bout, east float64) {
		switch won := b.Winner == domain.East; {
		case b.Basho.Compare(s.EvalFrom) >= 0:
			eval.add(east, won)
		case b.Basho.Compare(s.TrainFrom) >= 0:
			train.add(east, won)
		}
	})
	return Result{Params: p, Train: train.metrics(), Eval: eval.metrics()}
}

func upTo(tournaments [][]domain.Bout, through domain.BashoID) [][]domain.Bout {
	n, _ := slices.BinarySearchFunc(tournaments, through, func(bouts []domain.Bout, id domain.BashoID) int {
		if len(bouts) == 0 {
			return -1
		}
		return bouts[0].Basho.Compare(id)
	})
	if n < len(tournaments) && len(tournaments[n]) > 0 && tournaments[n][0].Basho == through {
		n++
	}
	return tournaments[:n]
}

// Grid returns the parameter sets to search (SPEC Appendix F).
func Grid() []Params {
	var out []Params
	for _, k := range []float64{8, 10, 12, 16, 20, 24, 32} {
		for _, kNew := range []float64{24, 32, 40, 48} {
			for _, newBouts := range []int{15, 30, 45} {
				for _, r := range []float64{0, 0.1, 0.2, 0.3, 0.5} {
					for _, scale := range []float64{0.5, 1, 1.5} {
						out = append(out, Params{K: k, KNew: kNew, NewBouts: newBouts, R: r, WarmupScale: scale})
					}
				}
			}
		}
	}
	return out
}

// Calibrate evaluates every parameter set in parallel and returns the results
// best first by training log loss, ties to the smaller K, then grid order.
// Tournaments should be in order of play within each (derive.ComparePlay),
// or every run sorts them again.
func Calibrate(grid []Params, tournaments [][]domain.Bout, s Split) []Result {
	results := make([]Result, len(grid))
	next := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range next {
				results[i] = Evaluate(grid[i], tournaments, s)
			}
		})
	}
	for i := range grid {
		next <- i
	}
	close(next)
	wg.Wait()
	slices.SortStableFunc(results, func(a, b Result) int {
		return cmp.Or(cmp.Compare(a.Train.LogLoss, b.Train.LogLoss), cmp.Compare(a.Params.K, b.Params.K))
	})
	return results
}

// Baselines are what the rating has to beat: a coin flip, and the higher
// rank winning with a chance fitted on the training years by rank gap.
type Baselines struct {
	Coin      Result
	Rank      Result
	RankSlope float64 // per rank level (derive.RankLevel), logistic
}

// FitBaselines scores both baselines on the bouts the model rates.
func FitBaselines(tournaments [][]domain.Bout, s Split) Baselines {
	type rated struct {
		gap  float64 // positive when east ranks higher
		won  bool
		eval bool
	}
	var bouts []rated
	Replay(Params{}, upTo(tournaments, s.Through), func(b domain.Bout, _ float64) {
		if b.Basho.Compare(s.TrainFrom) >= 0 {
			gap := float64(derive.RankLevel(b.West.Rank) - derive.RankLevel(b.East.Rank))
			bouts = append(bouts, rated{gap: gap, won: b.Winner == domain.East, eval: b.Basho.Compare(s.EvalFrom) >= 0})
		}
	})
	score := func(p func(gap float64) float64) (train, eval Metrics) {
		var t, e scorer
		for _, b := range bouts {
			if b.eval {
				e.add(p(b.gap), b.won)
			} else {
				t.add(p(b.gap), b.won)
			}
		}
		return t.metrics(), e.metrics()
	}
	logistic := func(slope float64) func(float64) float64 {
		return func(gap float64) float64 { return 1 / (1 + math.Exp(-slope*gap)) }
	}
	slope := goldenMin(0, 1, func(slope float64) float64 {
		train, _ := score(logistic(slope))
		return train.LogLoss
	})
	var out Baselines
	out.Coin.Train, out.Coin.Eval = score(func(float64) float64 { return 0.5 })
	out.Rank.Train, out.Rank.Eval = score(logistic(slope))
	out.RankSlope = slope
	return out
}

// goldenMin finds the minimum of a unimodal f on [a, b].
func goldenMin(a, b float64, f func(float64) float64) float64 {
	const phi = 0.6180339887498949
	c, d := b-phi*(b-a), a+phi*(b-a)
	fc, fd := f(c), f(d)
	for b-a > 1e-6 {
		if fc < fd {
			b, d, fd = d, c, fc
			c = b - phi*(b-a)
			fc = f(c)
		} else {
			a, c, fc = c, d, fd
			d = a + phi*(b-a)
			fd = f(d)
		}
	}
	return (a + b) / 2
}

// File is data/rating/params.json: the chosen parameters, whether the
// rating is shown (V9), and the evaluation scores the regression test pins.
type File struct {
	Schema    int     `json:"schema"`
	Generator string  `json:"generator"`
	Params    Params  `json:"params"`
	Show      bool    `json:"show"` // beats the rank baseline on evaluation log loss
	Split     Split   `json:"split"`
	Eval      Metrics `json:"eval"`
	RankEval  Metrics `json:"rankEval"`
	RankSlope float64 `json:"rankSlope"`
}

// FileSchema is the version of File.
const FileSchema = 1

// ReadFile decodes params.json.
func ReadFile(b []byte) (File, error) {
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("rating params: %w", err)
	}
	if f.Schema != FileSchema {
		return File{}, fmt.Errorf("rating params: schema %d, want %d", f.Schema, FileSchema)
	}
	return f, nil
}
