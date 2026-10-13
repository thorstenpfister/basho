package snapshot

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// mapSnapshot builds an in-memory snapshot holding tournaments. (Tests in
// other packages use snapshottest.MapFS, which this package cannot import.)
func mapSnapshot(t *testing.T, tournaments ...Tournament) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	m := Manifest{Schema: Schema, Files: map[string]string{}}
	for _, tour := range tournaments {
		data, err := EncodeTournament(tour)
		if err != nil {
			t.Fatal(err)
		}
		fsys[TournamentPath(tour.Basho.ID)] = &fstest.MapFile{Data: data}
		m.Basho = append(m.Basho, tour.Basho.ID)
	}
	idx, _ := EncodeRikishi([]domain.Rikishi{{ID: 8850, Name: domain.Name{En: "Onosato"}}})
	fsys[RikishiPath] = &fstest.MapFile{Data: idx}
	man, _ := EncodeManifest(m)
	fsys[ManifestPath] = &fstest.MapFile{Data: man}
	return fsys
}

func TestSnapshotReads(t *testing.T) {
	aki := recorded(t, aki2026, map[domain.Division][]int{domain.DivMakuuchi: {1, 13}, domain.DivJuryo: {13}})
	s, err := Open(mapSnapshot(t, aki))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(aki2026) || s.Has(aki2026.Next()) {
		t.Error("Has")
	}

	start := time.Now()
	b, err := s.Basho(aki2026)
	if took := time.Since(start); err != nil || took > 100*time.Millisecond {
		t.Errorf("Basho: %v after %v; want a tournament in under 100 ms (Phase 3 AC)", err, took)
	}
	if !reflect.DeepEqual(b, aki.Basho) {
		t.Errorf("Basho = %+v", b)
	}
	banzuke, _ := s.Banzuke(aki2026, domain.DivJuryo)
	if !reflect.DeepEqual(banzuke, aki.Banzuke[domain.DivJuryo]) {
		t.Error("Banzuke differs")
	}
	day13, _ := s.Torikumi(aki2026, domain.DivMakuuchi, 13)
	var want []domain.Bout
	for _, b := range aki.Bouts {
		if b.Day == 13 && b.Division == domain.DivMakuuchi {
			want = append(want, b)
		}
	}
	if len(want) == 0 || !reflect.DeepEqual(day13, want) {
		t.Errorf("Torikumi day 13: %d bouts, want %d", len(day13), len(want))
	}
	if _, err := s.Basho(aki2026.Next()); !errors.Is(err, ErrNotInSnapshot) {
		t.Errorf("a tournament not in the snapshot: %v", err)
	}
	if rs, err := s.Rikishi(); err != nil || len(rs) != 1 {
		t.Errorf("Rikishi = %v, %v", rs, err)
	}
}

func TestSnapshotKeepsTheLastTournament(t *testing.T) {
	a := Tournament{Basho: domain.Basho{ID: aki2026, Yusho: map[domain.Division]domain.RikishiID{}}}
	b := Tournament{Basho: domain.Basho{ID: aki2026.Next(), Yusho: map[domain.Division]domain.RikishiID{}}}
	s, err := Open(mapSnapshot(t, a, b))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []domain.BashoID{aki2026, aki2026, aki2026.Next()} {
		if _, err := s.Tournament(id); err != nil {
			t.Fatal(err)
		}
	}
	if s.last.Basho.ID != aki2026.Next() {
		t.Errorf("kept %v", s.last.Basho.ID)
	}
}
