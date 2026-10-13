package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"sync"

	"github.com/thorstenpfister/basho/internal/domain"
)

// ErrNotInSnapshot is returned for a tournament the snapshot does not hold:
// the next data layer has to answer (SPEC §5.8).
var ErrNotInSnapshot = errors.New("not in the snapshot")

// Snapshot reads finished tournaments from an embedded (or any) snapshot. It
// is safe for concurrent use.
type Snapshot struct {
	fsys     fs.FS
	manifest Manifest

	mu   sync.Mutex
	last Tournament // the most recently read: browsing stays within one

	rikishi func() ([]domain.Rikishi, error)
}

// Open opens the snapshot in fsys after checking its manifest.
func Open(fsys fs.FS) (*Snapshot, error) {
	m, err := ReadManifest(fsys)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{fsys: fsys, manifest: m}
	s.rikishi = sync.OnceValues(func() ([]domain.Rikishi, error) {
		if len(m.Basho) == 0 {
			return nil, nil
		}
		b, err := fs.ReadFile(fsys, RikishiPath)
		if err != nil {
			return nil, err
		}
		return DecodeRikishi(b)
	})
	return s, nil
}

// Manifest returns the snapshot's manifest.
func (s *Snapshot) Manifest() Manifest { return s.manifest }

// Has reports whether the snapshot holds tournament id.
func (s *Snapshot) Has(id domain.BashoID) bool { return s.manifest.Has(id) }

// Tournament returns a whole tournament, decoding it on first use.
func (s *Snapshot) Tournament(id domain.BashoID) (Tournament, error) {
	if !s.Has(id) {
		return Tournament{}, fmt.Errorf("%s: %w", id, ErrNotInSnapshot)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last.Basho.ID == id {
		return s.last, nil
	}
	t, err := ReadTournament(s.fsys, id)
	if err != nil {
		return Tournament{}, err
	}
	s.last = t
	return t, nil
}

// Basho returns a tournament's metadata.
func (s *Snapshot) Basho(id domain.BashoID) (domain.Basho, error) {
	t, err := s.Tournament(id)
	return t.Basho, err
}

// Banzuke returns a division's banzuke.
func (s *Snapshot) Banzuke(id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	t, err := s.Tournament(id)
	return t.Banzuke[div], err
}

// Torikumi returns a division's bouts of one day, playoffs included, in
// match order.
func (s *Snapshot) Torikumi(id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	t, err := s.Tournament(id)
	if err != nil {
		return nil, err
	}
	var out []domain.Bout
	for _, b := range t.Bouts {
		if b.Day == day && b.Division == div {
			out = append(out, b)
		}
	}
	return out, nil
}

// Rikishi returns the wrestler index, decoding it on first use.
func (s *Snapshot) Rikishi() ([]domain.Rikishi, error) { return s.rikishi() }
