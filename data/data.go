// Package data embeds the data shipped inside basho: the snapshot of finished
// tournaments (SPEC §4.3) and the rating parameters (§6.8). go:embed cannot reach outside a package's
// directory, so the embedding lives next to the files.
package data

import (
	"embed"
	"io/fs"

	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

//go:embed snapshot rating/params.json
var files embed.FS

// Snapshot opens the snapshot compiled into the binary. Only binaries that
// call it carry the data; basho-data builds snapshots and does not.
func Snapshot() (*snapshot.Snapshot, error) {
	sub, err := SnapshotFS()
	if err != nil {
		return nil, err
	}
	return snapshot.Open(sub)
}

// SnapshotFS returns the embedded snapshot's files, for tests that build a
// snapshot from part of them.
func SnapshotFS() (fs.FS, error) { return fs.Sub(files, "snapshot") }

// RatingParams returns the calibrated rating parameters compiled into the
// binary, with whether the rating is shown (V9).
func RatingParams() (rating.File, error) {
	b, err := files.ReadFile("rating/params.json")
	if err != nil {
		return rating.File{}, err
	}
	return rating.ReadFile(b)
}
