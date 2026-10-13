// Package snapshottest builds in-memory snapshots for tests.
package snapshottest

import (
	"testing/fstest"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

// MapFS returns a complete snapshot holding tournaments and wrestlers, with
// checksums and the index, as snapshot.Open and verify expect it.
func MapFS(tournaments []snapshot.Tournament, wrestlers []domain.Rikishi) (fstest.MapFS, error) {
	fsys := fstest.MapFS{}
	m := snapshot.Manifest{Schema: snapshot.Schema, Source: "test", Files: map[string]string{}}
	add := func(path string, data []byte) {
		fsys[path] = &fstest.MapFile{Data: data}
		m.Files[path] = snapshot.Checksum(data)
	}
	for _, t := range tournaments {
		data, err := snapshot.EncodeTournament(t)
		if err != nil {
			return nil, err
		}
		add(snapshot.TournamentPath(t.Basho.ID), data)
		m.Basho = append(m.Basho, t.Basho.ID)
	}
	index, err := snapshot.EncodeRikishi(wrestlers)
	if err != nil {
		return nil, err
	}
	add(snapshot.RikishiPath, index)
	m.IndexThrough = m.Newest()
	manifest, err := snapshot.EncodeManifest(m)
	if err != nil {
		return nil, err
	}
	fsys[snapshot.ManifestPath] = &fstest.MapFile{Data: manifest}
	return fsys, nil
}
