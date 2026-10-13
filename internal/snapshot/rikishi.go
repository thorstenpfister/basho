package snapshot

import (
	"cmp"
	"slices"

	"github.com/thorstenpfister/basho/internal/domain"
)

type rikishiFile struct {
	versioned
	Rikishi []rikishiEntry `json:"rikishi"`
}

type rikishiEntry struct {
	ID       domain.RikishiID `json:"id"`
	NSKID    int              `json:"nsk,omitempty"`
	SumoDBID int              `json:"sumodb,omitempty"`
	En       string           `json:"en"`
	Jp       string           `json:"jp,omitempty"`
	Kana     string           `json:"kana,omitempty"`
	Heya     string           `json:"heya,omitempty"`
	Shusshin string           `json:"shusshin,omitempty"`
	Birth    domain.Date      `json:"birth"`
	Debut    domain.BashoID   `json:"debut"`
	Height   float64          `json:"cm,omitempty"`
	Weight   float64          `json:"kg,omitempty"`
	Retired  bool             `json:"retired,omitempty"`
}

// EncodeRikishi returns the wrestler index file's content, sorted by id.
func EncodeRikishi(rs []domain.Rikishi) ([]byte, error) {
	f := rikishiFile{versioned: versioned{Schema}, Rikishi: make([]rikishiEntry, 0, len(rs))}
	for _, r := range rs {
		f.Rikishi = append(f.Rikishi, rikishiEntry{
			ID: r.ID, NSKID: r.NSKID, SumoDBID: r.SumoDBID, En: r.Name.En, Jp: r.Name.Jp, Kana: r.Name.Kana,
			Heya: r.Heya, Shusshin: r.Shusshin, Birth: r.Birth, Debut: r.Debut,
			Height: r.Height, Weight: r.Weight, Retired: r.Retired,
		})
	}
	slices.SortFunc(f.Rikishi, func(a, b rikishiEntry) int { return cmp.Compare(a.ID, b.ID) })
	return encode(f)
}

// DecodeRikishi parses the wrestler index file.
func DecodeRikishi(b []byte) ([]domain.Rikishi, error) {
	var f rikishiFile
	if err := decode(b, &f); err != nil {
		return nil, err
	}
	if err := f.check(); err != nil {
		return nil, err
	}
	out := make([]domain.Rikishi, 0, len(f.Rikishi))
	for _, e := range f.Rikishi {
		out = append(out, domain.Rikishi{
			ID: e.ID, NSKID: e.NSKID, SumoDBID: e.SumoDBID, Name: domain.Name{En: e.En, Jp: e.Jp, Kana: e.Kana},
			Heya: e.Heya, Shusshin: e.Shusshin, Birth: e.Birth, Debut: e.Debut,
			Height: e.Height, Weight: e.Weight, Retired: e.Retired,
		})
	}
	return out, nil
}
