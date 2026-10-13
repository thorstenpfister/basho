// Package data embeds the data shipped inside basho: the snapshot of finished
// tournaments (SPEC §4.3). go:embed cannot reach outside a package's
// directory, so the embedding lives next to the files.
package data

import (
	"embed"
	"io/fs"

	"github.com/thorstenpfister/basho/internal/snapshot"
)

//go:embed snapshot
var files embed.FS

// Snapshot opens the snapshot compiled into the binary. Only binaries that
// call it carry the data; basho-data builds snapshots and does not.
func Snapshot() (*snapshot.Snapshot, error) {
	sub, err := fs.Sub(files, "snapshot")
	if err != nil {
		return nil, err
	}
	return snapshot.Open(sub)
}
