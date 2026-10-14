package idmap

import (
	"context"
	"sync"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// RefreshInterval is how often an unknown JSA ID may refresh the wrestler
// index (SPEC §5.5).
const RefreshInterval = time.Hour

// Map maps JSA IDs (sumo-api's nskId, osada.us's id) to sumo-api IDs
// (SPEC §5.5). It is safe for concurrent use.
type Map struct {
	mu    sync.RWMutex
	byNSK map[int]domain.Rikishi

	refresh   func(context.Context) ([]domain.Rikishi, error)
	now       func() time.Time
	refreshMu sync.Mutex // one refresh at a time
	refreshed time.Time  // the last refresh, successful or not
}

// New returns a map over the wrestlers.
func New(rs []domain.Rikishi) *Map {
	m := &Map{byNSK: map[int]domain.Rikishi{}, now: time.Now}
	m.Add(rs)
	return m
}

// SetRefresh sets how an unknown JSA ID is looked up: fetch returns the
// wrestlers to add (sumo-api's active ones), at most once per
// RefreshInterval by now.
func (m *Map) SetRefresh(fetch func(context.Context) ([]domain.Rikishi, error), now func() time.Time) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.refresh, m.now = fetch, now
}

// Add adds or updates wrestlers; those without a JSA ID are skipped.
func (m *Map) Add(rs []domain.Rikishi) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range rs {
		if r.NSKID != 0 {
			m.byNSK[r.NSKID] = r
		}
	}
}

// ByJSA returns the sumo-api ID of a JSA ID.
func (m *Map) ByJSA(nsk int) (domain.RikishiID, bool) {
	r, ok := m.lookup(nsk)
	return r.ID, ok
}

func (m *Map) lookup(nsk int) (domain.Rikishi, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.byNSK[nsk]
	return r, ok
}

// Len returns the number of mapped wrestlers.
func (m *Map) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byNSK)
}

// Identity is a wrestler osada.us names by JSA ID, as basho knows them.
type Identity struct {
	ID   domain.RikishiID
	Name domain.Name
	// Temporary: sumo-api does not know them yet. ID is TemporaryID, the
	// romaji name is transliterated (Name.Approx), and profile fields are
	// unknown.
	Temporary bool
}

// TemporaryID is the ID of a wrestler sumo-api does not know yet: the JSA ID,
// negated, so it never collides with a sumo-api ID.
func TemporaryID(nsk int) domain.RikishiID { return domain.RikishiID(-nsk) }

// Resolve maps a JSA ID to a sumo-api wrestler. An unknown ID refreshes the
// index first, at most once an hour; still unknown, it is a temporary
// identity named from jp and the kana reading. A known wrestler keeps the
// index's romaji with jp and kana from osada.us.
func (m *Map) Resolve(ctx context.Context, nsk int, jp, kana string) Identity {
	r, ok := m.lookup(nsk)
	if !ok && m.refreshNow(ctx) {
		r, ok = m.lookup(nsk)
	}
	if !ok {
		return Identity{ID: TemporaryID(nsk), Name: domain.Name{En: Romaji(kana), Jp: jp, Kana: kana, Approx: true}, Temporary: true}
	}
	name := r.Name
	if jp != "" {
		name.Jp = jp
	}
	name.Kana = kana
	return Identity{ID: r.ID, Name: name}
}

// refreshNow refreshes the index unless it was refreshed within the
// interval; it reports whether it added wrestlers.
func (m *Map) refreshNow(ctx context.Context) bool {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if m.refresh == nil || !m.refreshed.IsZero() && m.now().Sub(m.refreshed) < RefreshInterval {
		return false
	}
	m.refreshed = m.now()
	rs, err := m.refresh(ctx)
	if err != nil {
		return false
	}
	m.Add(rs)
	return len(rs) > 0
}
