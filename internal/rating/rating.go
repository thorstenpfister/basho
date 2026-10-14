package rating

import (
	"errors"
	"math"
	"slices"
	"sort"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// Base is the centre of the rating scale.
const Base = 1500

// Params are the model's tuning, found by calibration (SPEC §6.8, Appendix F).
type Params struct {
	K           float64 `json:"k"`           // update factor once established
	KNew        float64 `json:"kNew"`        // update factor for the first NewBouts bouts
	NewBouts    int     `json:"newBouts"`    // sekitori bouts counted as new
	R           float64 `json:"r"`           // pull towards the band average per missed tournament
	WarmupScale float64 `json:"warmupScale"` // spread of the warm-up table around Base
}

// Starting ratings come from the band's average over this many earlier
// tournaments, once that holds at least minBandSample established wrestlers;
// before, from the warm-up table.
const (
	bandWindow    = 6
	minBandSample = 10
)

// Band groups ranks for starting ratings and the pull after absences.
type Band int

const (
	BandY Band = iota
	BandO
	BandS
	BandK
	BandM1  // Maegashira 1–5
	BandM6  // 6–10
	BandM11 // 11 and below
	BandJ1  // Juryo 1–7
	BandJ8  // 8 and below
	BandMs  // Makushita, visiting a Juryo bout
	bands
)

// warmup is the starting rating per band for the 1960 warm-up period and
// whenever a band has too little history (SPEC Appendix F).
var warmup = [bands]float64{1750, 1700, 1650, 1625, 1575, 1550, 1525, 1475, 1450, 1425}

// BandOf returns the band of a rank.
func BandOf(r domain.Rank) Band {
	switch r.Title {
	case domain.Yokozuna:
		return BandY
	case domain.Ozeki:
		return BandO
	case domain.Sekiwake:
		return BandS
	case domain.Komusubi:
		return BandK
	case domain.Maegashira:
		return [...]Band{BandM1, BandM6, BandM11}[min((max(r.Number, 1)-1)/5, 2)]
	case domain.Juryo:
		if r.Number <= 7 {
			return BandJ1
		}
		return BandJ8
	default:
		return BandMs
	}
}

// WinChance is the probability that a wrestler rated a beats one rated b.
func WinChance(a, b float64) float64 { return 1 / (1 + math.Pow(10, (b-a)/400)) }

// Ratings runs the model bout by bout in order of play over sekitori bouts
// and answers as-of queries. Tournaments are added oldest first; the newest
// can be replaced as live results come in. It is not safe for concurrent
// use while being changed.
type Ratings struct {
	p           Params
	keepHistory bool // for Before
	replaceable bool // the newest tournament can be set again
	observe     func(domain.Bout, float64)

	wrestlers   map[domain.RikishiID]*wrestler
	tournaments []domain.BashoID
	bandSums    map[domain.BashoID]*[bands]bandSum
	saved       checkpoint // state before the newest tournament
}

type wrestler struct {
	rating  float64
	bouts   int            // rated bouts so far
	last    domain.BashoID // the latest tournament with a rated bout
	history []point
}

// point is a wrestler's rating after one bout, placed in the order of play.
type point struct {
	basho   domain.BashoID
	day     int16
	match   int16
	div     domain.Division
	playoff bool
	after   float64
}

type bandSum struct {
	sum float64
	n   int
}

type checkpoint struct {
	basho     domain.BashoID
	wrestlers map[domain.RikishiID]state
}

type state struct {
	rating  float64
	bouts   int
	last    domain.BashoID
	history int
}

// New returns an empty model.
func New(p Params) *Ratings {
	return &Ratings{
		p: p, keepHistory: true, replaceable: true,
		wrestlers: map[domain.RikishiID]*wrestler{},
		bandSums:  map[domain.BashoID]*[bands]bandSum{},
	}
}

// ErrOutOfOrder is returned for a tournament older than the newest added.
var ErrOutOfOrder = errors.New("rating: tournament older than the newest one added")

// Set adds tournament basho's bouts, or replaces them if basho is the newest
// tournament added so far.
func (r *Ratings) Set(basho domain.BashoID, bouts []domain.Bout) error {
	if n := len(r.tournaments); n > 0 {
		switch c := basho.Compare(r.tournaments[n-1]); {
		case c < 0:
			return ErrOutOfOrder
		case c == 0:
			r.restore()
		}
	}
	if len(r.tournaments) == 0 || r.tournaments[len(r.tournaments)-1] != basho {
		if r.replaceable {
			r.save(basho)
		}
		r.tournaments = append(r.tournaments, basho)
	}
	if !slices.IsSortedFunc(bouts, derive.ComparePlay) {
		bouts = slices.Clone(bouts)
		slices.SortStableFunc(bouts, derive.ComparePlay)
	}
	for i := range bouts {
		r.play(&bouts[i])
	}
	return nil
}

// play updates both wrestlers for one bout. Fusen and draws are skipped;
// playoffs count.
func (r *Ratings) play(b *domain.Bout) {
	if b.Winner == 0 || b.Kimarite.Category() == domain.NoTechnique {
		return
	}
	east, west := r.enter(b.East, b.Basho), r.enter(b.West, b.Basho)
	pEast := WinChance(east.rating, west.rating)
	if r.observe != nil {
		r.observe(*b, pEast)
	}
	won := 0.0
	if b.Winner == domain.East {
		won = 1
	}
	east.rating += r.k(east) * (won - pEast)
	west.rating += r.k(west) * (pEast - won) // (1-won) - (1-pEast)
	for _, w := range []*wrestler{east, west} {
		w.bouts++
		if r.keepHistory {
			w.history = append(w.history, point{
				basho: b.Basho, day: int16(b.Day), match: int16(b.MatchNo), div: b.Division, playoff: b.IsPlayoff, after: w.rating,
			})
		}
	}
}

func (r *Ratings) k(w *wrestler) float64 {
	if w.bouts < r.p.NewBouts {
		return r.p.KNew
	}
	return r.p.K
}

// enter returns a wrestler ready for a bout in tournament basho: on their
// first bout there, a debut starts from the band's rating and a return after
// missed tournaments is pulled towards it; established wrestlers' ratings
// feed the band averages.
func (r *Ratings) enter(side domain.BoutSide, basho domain.BashoID) *wrestler {
	w := r.wrestlers[side.RikishiID]
	if w != nil && w.last == basho {
		return w
	}
	band := BandOf(side.Rank)
	if w == nil {
		w = &wrestler{rating: r.start(basho, band)}
		r.wrestlers[side.RikishiID] = w
	} else {
		w.rating = r.pull(w.rating, w.last, basho, band)
		sums := r.bandSums[basho]
		if sums == nil {
			sums = &[bands]bandSum{}
			r.bandSums[basho] = sums
		}
		sums[band].sum += w.rating
		sums[band].n++
	}
	w.last = basho
	return w
}

// pull moves a rating last earned in tournament from towards the band's
// average, by R for each tournament held in between.
func (r *Ratings) pull(rating float64, from, to domain.BashoID, band Band) float64 {
	missed := 0
	for id := from.Next(); id.Compare(to) < 0; id = id.Next() {
		if id.Held() != domain.NotHeld {
			missed++
		}
	}
	if missed == 0 || r.p.R == 0 {
		return rating
	}
	avg := r.start(to, band)
	return avg + (rating-avg)*math.Pow(1-r.p.R, float64(missed))
}

// start is the starting rating in a band for tournament basho: the average of
// its established wrestlers over the bandWindow tournaments before basho, or
// the warm-up table.
func (r *Ratings) start(basho domain.BashoID, band Band) float64 {
	i, _ := slices.BinarySearchFunc(r.tournaments, basho, domain.BashoID.Compare)
	var sum bandSum
	for _, id := range r.tournaments[max(i-bandWindow, 0):i] {
		if s := r.bandSums[id]; s != nil {
			sum.sum += s[band].sum
			sum.n += s[band].n
		}
	}
	if sum.n < minBandSample {
		return Base + r.p.WarmupScale*(warmup[band]-Base)
	}
	return sum.sum / float64(sum.n)
}

// save records the state before tournament basho, so Set can replay it.
func (r *Ratings) save(basho domain.BashoID) {
	cp := checkpoint{basho: basho, wrestlers: make(map[domain.RikishiID]state, len(r.wrestlers))}
	for id, w := range r.wrestlers {
		cp.wrestlers[id] = state{rating: w.rating, bouts: w.bouts, last: w.last, history: len(w.history)}
	}
	r.saved = cp
}

// restore returns to the state before the newest tournament.
func (r *Ratings) restore() {
	for id, w := range r.wrestlers {
		s, ok := r.saved.wrestlers[id]
		if !ok {
			delete(r.wrestlers, id) // debuted in the replaced tournament
			continue
		}
		w.rating, w.bouts, w.last, w.history = s.rating, s.bouts, s.last, w.history[:s.history]
	}
	delete(r.bandSums, r.saved.basho)
}

// Before returns id's rating before the cutoff, ranked rank in the cutoff's
// tournament. Without a rated bout before the cutoff it is the starting
// rating for rank's band, and provisional is true. After missed tournaments
// it includes the pull the next bout would apply.
func (r *Ratings) Before(id domain.RikishiID, rank domain.Rank, c derive.AsOf) (rating float64, provisional bool) {
	w := r.wrestlers[id]
	if w == nil {
		return r.start(c.Basho, BandOf(rank)), true
	}
	n := sort.Search(len(w.history), func(i int) bool { return !c.Includes(w.history[i].bout()) })
	if n == 0 {
		return r.start(c.Basho, BandOf(rank)), true
	}
	p := w.history[n-1]
	if _, err := domain.NewBashoID(c.Basho.Year, c.Basho.Month); err != nil {
		return p.after, false // NoCutoff: no tournament to return in, the latest rating
	}
	return r.pull(p.after, p.basho, c.Basho, BandOf(rank)), false
}

func (p point) bout() domain.Bout {
	return domain.Bout{Basho: p.basho, Day: int(p.day), MatchNo: int(p.match), Division: p.div, IsPlayoff: p.playoff}
}

// Replay runs the model over tournaments, oldest first, without keeping
// history or checkpoints, and reports each rated bout with the east side's win chance
// before it: what calibration needs.
func Replay(p Params, tournaments [][]domain.Bout, observe func(b domain.Bout, east float64)) {
	r := New(p)
	r.keepHistory, r.replaceable, r.observe = false, false, observe
	for _, bouts := range tournaments {
		if len(bouts) > 0 {
			_ = r.Set(bouts[0].Basho, bouts) // in order by contract
		}
	}
}
