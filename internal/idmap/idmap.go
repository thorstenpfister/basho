package idmap

import (
	"sync"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Map maps JSA IDs (sumo-api's nskId, osada.us's id) to sumo-api IDs
// (SPEC §5.5). It is safe for concurrent use. Phase 9 adds the refresh for
// unknown IDs and the temporary entries for newcomers.
type Map struct {
	mu    sync.RWMutex
	byNSK map[int]domain.RikishiID
}

// New returns a map over the wrestlers.
func New(rs []domain.Rikishi) *Map {
	m := &Map{byNSK: map[int]domain.RikishiID{}}
	m.Add(rs)
	return m
}

// Add adds or updates wrestlers; those without a JSA ID are skipped.
func (m *Map) Add(rs []domain.Rikishi) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range rs {
		if r.NSKID != 0 {
			m.byNSK[r.NSKID] = r.ID
		}
	}
}

// ByJSA returns the sumo-api ID of a JSA ID.
func (m *Map) ByJSA(nsk int) (domain.RikishiID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.byNSK[nsk]
	return id, ok
}

// Len returns the number of mapped wrestlers.
func (m *Map) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byNSK)
}
