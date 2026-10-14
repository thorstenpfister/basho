package spoiler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/thorstenpfister/basho/internal/atomicfile"
	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// Horizon is the last bout seen in a tournament, as a point in the order of
// play (SPEC §9). One horizon covers both divisions: a revealed Makuuchi
// bout also reveals the Juryo bouts fought before it that day, as watching
// the broadcast does, so every derived number has one consistent cutoff.
type Horizon struct {
	Day      int             `json:"day"`                // 0: nothing seen yet
	Division domain.Division `json:"division,omitempty"` // of the bout; zero with Day alone: the whole day
	MatchNo  int             `json:"match,omitempty"`
	Playoff  bool            `json:"playoff,omitempty"`
}

// AsOf is the horizon as a cutoff: everything up to and including the bout.
func (h Horizon) AsOf(id domain.BashoID) derive.AsOf {
	if h.MatchNo == 0 {
		if h.Day == 0 {
			return derive.BeforeDay(id, 1)
		}
		return derive.AfterDay(id, h.Day)
	}
	return derive.AsOf{Basho: id, Day: h.Day, Division: h.Division, MatchNo: h.MatchNo, Playoff: h.Playoff, Inclusive: true}
}

// String is "Day 12 · bout 14", "Day 12" or "before Day 1".
func (h Horizon) String() string {
	switch {
	case h.Day == 0:
		return "before Day 1"
	case h.MatchNo == 0:
		return fmt.Sprintf("Day %d", h.Day)
	case h.Playoff:
		return fmt.Sprintf("Day %d · %s playoff %d", h.Day, h.Division, h.MatchNo)
	default:
		return fmt.Sprintf("Day %d · %s bout %d", h.Day, h.Division, h.MatchNo)
	}
}

// Of returns the horizon through a bout.
func Of(b domain.Bout) Horizon {
	return Horizon{Day: b.Day, Division: b.Division, MatchNo: b.MatchNo, Playoff: b.IsPlayoff}
}

// State holds the horizons of every tournament and saves them to the state
// file ($XDG_STATE_HOME/basho/state.json). It is safe for concurrent use.
type State struct {
	path string // empty: in memory only (tests, --no-spoilers without a file)

	mu       sync.Mutex
	horizons map[domain.BashoID]Horizon
}

// FileName is the state file in basho's state directory.
const FileName = "state.json"

// fileSchema is the state file's version.
const fileSchema = 1

type file struct {
	V        int                `json:"v"`
	Horizons map[string]Horizon `json:"horizons"`
}

// Load reads the state file at path; a missing file is an empty state.
func Load(path string) (*State, error) {
	s := &State{path: path, horizons: map[domain.BashoID]Horizon{}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.V != fileSchema {
		return nil, fmt.Errorf("%s: schema %d, want %d", path, f.V, fileSchema)
	}
	for k, h := range f.Horizons {
		id, err := domain.ParseBashoID(k)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		s.horizons[id] = h
	}
	return s, nil
}

// Horizon returns a tournament's horizon; a tournament never revealed starts
// before Day 1.
func (s *State) Horizon(id domain.BashoID) Horizon {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.horizons[id]
}

// Cutoff returns a tournament's horizon as a cutoff.
func (s *State) Cutoff(id domain.BashoID) derive.AsOf { return s.Horizon(id).AsOf(id) }

// Visible reports whether a bout's result is within the horizon.
func (s *State) Visible(b domain.Bout) bool { return s.Cutoff(b.Basho).Includes(b) }

// Reveal moves a tournament's horizon through b if that is later, and saves
// it; it reports whether the horizon moved.
func (s *State) Reveal(b domain.Bout) (bool, error) {
	return s.RevealTo(b.Basho, Of(b))
}

// RevealTo moves a tournament's horizon to h if that is later (`ctrl+r`,
// `--reveal-to`), and saves it.
func (s *State) RevealTo(id domain.BashoID, h Horizon) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !h.AsOf(id).After(s.horizons[id].AsOf(id)) {
		return false, nil
	}
	s.horizons[id] = h
	return true, s.save()
}

// Set replaces a tournament's horizon, earlier or later: `--reveal-to` on
// the command line means exactly there.
func (s *State) Set(id domain.BashoID, h Horizon) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.horizons[id] = h
	return s.save()
}

// save writes the state file; the caller holds mu.
func (s *State) save() error {
	if s.path == "" {
		return nil
	}
	f := file{V: fileSchema, Horizons: map[string]Horizon{}}
	for id, h := range s.horizons {
		f.Horizons[id.String()] = h
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return atomicfile.Write(s.path, append(b, '\n'))
}
