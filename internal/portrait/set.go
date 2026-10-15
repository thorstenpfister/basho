package portrait

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"path"
	"slices"
	"strconv"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Sizes are the portraits' widths in pixels, largest first (SPEC §8.9):
// the VS screen shows the largest that fits.
var Sizes = []int{48, 32, 20}

// Changes says what was done to every photo, as its licence asks.
const Changes = "cropped, downscaled, colour-reduced"

// Credit is one photo's attribution, a line of credits.json.
type Credit struct {
	RikishiID  domain.RikishiID `json:"id"`
	Name       string           `json:"name"`
	File       string           `json:"file"` // on Commons
	Author     string           `json:"author"`
	Licence    string           `json:"licence"`
	LicenceURL string           `json:"licenceUrl,omitempty"`
	SourceURL  string           `json:"sourceUrl"`
	Year       int              `json:"year,omitempty"` // when the photo was taken
}

// Set is the portraits shipped with basho: data/portraits.
type Set struct {
	fsys    fs.FS
	credits []Credit // by name
	byID    map[domain.RikishiID]Credit
}

// Open reads a portrait set: credits.json, and <size>/<id>.png on demand.
func Open(fsys fs.FS) (*Set, error) {
	b, err := fs.ReadFile(fsys, "credits.json")
	if err != nil {
		return nil, err
	}
	s := &Set{fsys: fsys, byID: map[domain.RikishiID]Credit{}}
	if err := json.Unmarshal(b, &s.credits); err != nil {
		return nil, fmt.Errorf("portraits: credits.json: %w", err)
	}
	slices.SortFunc(s.credits, func(a, b Credit) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.RikishiID, b.RikishiID))
	})
	for _, c := range s.credits {
		s.byID[c.RikishiID] = c
	}
	return s, nil
}

// Empty is a set without portraits.
func Empty() *Set { return &Set{credits: []Credit{}, byID: map[domain.RikishiID]Credit{}} }

// Credits returns every photo's attribution, by name.
func (s *Set) Credits() []Credit { return s.credits }

// Credit returns the attribution of a wrestler's photo.
func (s *Set) Credit(id domain.RikishiID) (Credit, bool) {
	c, ok := s.byID[id]
	return c, ok
}

// Image returns a wrestler's portrait at size (one of Sizes).
func (s *Set) Image(id domain.RikishiID, size int) (*image.Paletted, error) {
	if _, ok := s.byID[id]; !ok {
		return nil, fs.ErrNotExist
	}
	b, err := fs.ReadFile(s.fsys, Path(id, size))
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("portraits: %s: %w", Path(id, size), err)
	}
	p, ok := img.(*image.Paletted)
	if !ok {
		return nil, errors.New("portraits: " + Path(id, size) + " is not paletted")
	}
	return p, nil
}

// Path is where a portrait lives in the set.
func Path(id domain.RikishiID, size int) string {
	return path.Join(strconv.Itoa(size), strconv.Itoa(int(id))+".png")
}
