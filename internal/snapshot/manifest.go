package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Schema is the snapshot format version. It is written into the manifest and
// every file; a loader refuses other versions.
const Schema = 1

// Paths inside a snapshot directory (SPEC §4.3).
const (
	ManifestPath = "manifest.json"
	RikishiPath  = "rikishi.json.zst"
)

// TournamentPath returns the file of one tournament, e.g. "basho/202609.json.zst".
func TournamentPath(id domain.BashoID) string { return "basho/" + id.String() + ".json.zst" }

// Manifest describes a snapshot: what it contains and how to check it. It is
// stored as indented JSON so updates show up as readable diffs.
type Manifest struct {
	Schema      int               `json:"schema"`
	Generator   string            `json:"generator"` // basho-data version
	GeneratedAt time.Time         `json:"generatedAt"`
	Source      string            `json:"source"`
	Basho       []domain.BashoID  `json:"basho"` // ascending
	Files       map[string]string `json:"files"` // path → SHA-256, hex
}

// Has reports whether the snapshot contains tournament id.
func (m Manifest) Has(id domain.BashoID) bool {
	_, ok := slices.BinarySearchFunc(m.Basho, id, domain.BashoID.Compare)
	return ok
}

// Newest returns the newest tournament, or the zero BashoID for an empty snapshot.
func (m Manifest) Newest() domain.BashoID {
	if len(m.Basho) == 0 {
		return domain.BashoID{}
	}
	return m.Basho[len(m.Basho)-1]
}

// EncodeManifest returns the manifest file's content.
func EncodeManifest(m Manifest) ([]byte, error) {
	b, err := json.MarshalIndent(m, "", "  ")
	return append(b, '\n'), err
}

// DecodeManifest parses a manifest and checks its schema version.
func DecodeManifest(b []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	if m.Schema != Schema {
		return Manifest{}, fmt.Errorf("manifest: schema %d, want %d", m.Schema, Schema)
	}
	return m, nil
}

// Checksum returns the hex SHA-256 recorded in the manifest.
func Checksum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
