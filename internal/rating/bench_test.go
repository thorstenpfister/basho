package rating_test

import (
	"testing"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
)

// load returns the embedded snapshot's tournaments, oldest first.
func load(b *testing.B) [][]domain.Bout {
	b.Helper()
	s, err := data.Snapshot()
	if err != nil {
		b.Fatal(err)
	}
	x, err := s.BuildIndex()
	if err != nil {
		b.Fatal(err)
	}
	var ts [][]domain.Bout
	for _, id := range x.Tournaments() {
		ts = append(ts, x.Tournament(id))
	}
	return ts
}

// p is a mid-grid parameter set.
var p = rating.Params{K: 20, KNew: 40, NewBouts: 30, R: 0.2, WarmupScale: 1}

func BenchmarkSet(b *testing.B) {
	ts := load(b)
	for b.Loop() {
		r := rating.New(p)
		for _, bouts := range ts {
			_ = r.Set(bouts[0].Basho, bouts)
		}
	}
}

func BenchmarkReplay(b *testing.B) {
	ts := load(b)
	for b.Loop() {
		rating.Replay(p, ts, func(domain.Bout, float64) {})
	}
}
