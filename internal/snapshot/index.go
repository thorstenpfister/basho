package snapshot

import (
	"cmp"
	"maps"
	"runtime"
	"slices"
	"sync"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Index holds bouts by tournament and by wrestler (SPEC §5.8): the source of
// head-to-head, moveset, last 5, yusho counts and ratings. The service builds
// it from the snapshot and extends it with later tournaments and live
// results. It is safe for concurrent use.
//
// Stored slices are never modified, only replaced (copy on write), so readers
// get them without copying: treat them as read-only.
type Index struct {
	mu        sync.RWMutex
	basho     map[domain.BashoID]domain.Basho
	byBasho   map[domain.BashoID][]domain.Bout
	byRikishi map[domain.RikishiID][]*domain.Bout // chronological
}

// NewIndex returns an empty index.
func NewIndex() *Index {
	return &Index{
		basho:     map[domain.BashoID]domain.Basho{},
		byBasho:   map[domain.BashoID][]domain.Bout{},
		byRikishi: map[domain.RikishiID][]*domain.Bout{},
	}
}

// Set replaces everything the index holds for tournament b.ID: a tournament
// newer than the snapshot, or the current one as results come in.
func (x *Index) Set(b domain.Basho, bouts []domain.Bout) {
	id := b.ID
	bouts = slices.Clone(bouts)
	SortBouts(bouts)
	x.mu.Lock()
	defer x.mu.Unlock()

	added := map[domain.RikishiID][]*domain.Bout{}
	for i := range bouts {
		p := &bouts[i]
		added[p.East.RikishiID] = append(added[p.East.RikishiID], p)
		added[p.West.RikishiID] = append(added[p.West.RikishiID], p)
	}
	touched := maps.Clone(added)
	for _, old := range x.byBasho[id] {
		touched[old.East.RikishiID], touched[old.West.RikishiID] = nil, nil
	}
	for r := range touched {
		// A new slice: readers may hold the old one.
		career := slices.DeleteFunc(slices.Clone(x.byRikishi[r]), func(p *domain.Bout) bool { return p.Basho == id })
		career = append(career, added[r]...)
		if !slices.IsSortedFunc(career, compareChrono) { // tournaments mostly arrive in order
			slices.SortStableFunc(career, compareChrono)
		}
		x.byRikishi[r] = career
	}
	x.basho[id] = b
	x.byBasho[id] = bouts
}

// Basho returns a tournament's metadata (dates, yusho, sansho).
func (x *Index) Basho(id domain.BashoID) (domain.Basho, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	b, ok := x.basho[id]
	return b, ok
}

// Tournament returns the bouts of tournament id, in SortBouts order.
func (x *Index) Tournament(id domain.BashoID) []domain.Bout {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.byBasho[id]
}

// Rikishi returns a wrestler's sekitori bouts, oldest first.
func (x *Index) Rikishi(id domain.RikishiID) []*domain.Bout {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.byRikishi[id]
}

// RikishiBefore returns a wrestler's bouts before day of tournament basho
// (day 1: before the tournament), oldest first: the spoiler-safe view of a
// career as of a point in time (SPEC §6).
func (x *Index) RikishiBefore(id domain.RikishiID, basho domain.BashoID, day int) []*domain.Bout {
	career := x.Rikishi(id)
	n, _ := slices.BinarySearchFunc(career, &domain.Bout{Basho: basho, Day: day}, compareChrono)
	return career[:n]
}

// Tournaments returns the tournaments the index holds, oldest first.
func (x *Index) Tournaments() []domain.BashoID {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return slices.SortedFunc(maps.Keys(x.byBasho), domain.BashoID.Compare)
}

func compareChrono(a, b *domain.Bout) int {
	return cmp.Or(a.Basho.Compare(b.Basho), compareBouts(*a, *b))
}

// BuildIndex returns a new bout index over every tournament in the snapshot,
// decoding them in parallel. The caller owns it and extends it with Set.
func (s *Snapshot) BuildIndex() (*Index, error) {
	ids := s.manifest.Basho
	decoded := make([]Tournament, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	next := make(chan int)
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range next {
				decoded[i], errs[i] = ReadTournament(s.fsys, ids[i])
			}
		})
	}
	for i := range ids {
		next <- i
	}
	close(next)
	wg.Wait()

	// No other reader exists yet, so careers are appended to in place;
	// tournaments come in ascending order, so they stay chronological.
	x := NewIndex()
	for i, t := range decoded {
		if errs[i] != nil {
			return nil, errs[i]
		}
		x.basho[t.Basho.ID] = t.Basho
		x.byBasho[t.Basho.ID] = t.Bouts // already in SortBouts order
		for j := range t.Bouts {
			p := &t.Bouts[j]
			x.byRikishi[p.East.RikishiID] = append(x.byRikishi[p.East.RikishiID], p)
			x.byRikishi[p.West.RikishiID] = append(x.byRikishi[p.West.RikishiID], p)
		}
	}
	return x, nil
}
