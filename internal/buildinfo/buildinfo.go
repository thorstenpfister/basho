package buildinfo

import "runtime/debug"

// Set via -ldflags "-X github.com/thorstenpfister/basho/internal/buildinfo.Version=…" (and Commit, Date).
var (
	Version string
	Commit  string
	Date    string
)

// Info is the resolved build information.
type Info struct {
	Version string // release tag, module version, or "devel"
	Commit  string // short revision, "-dirty" if built from a modified tree; may be empty
	Date    string // build or commit time (RFC 3339); may be empty
}

const shortCommit = 12

// Get returns the build information: ldflags values first, then the VCS
// stamping of the Go toolchain.
func Get() Info {
	bi, _ := debug.ReadBuildInfo()
	return resolve(Info{Version: Version, Commit: Commit, Date: Date}, bi)
}

// resolve fills the fields ldflags left empty from bi.
func resolve(info Info, bi *debug.BuildInfo) Info {
	if bi == nil {
		bi = &debug.BuildInfo{}
	}
	var revision, modified string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			if info.Date == "" {
				info.Date = s.Value
			}
		case "vcs.modified":
			modified = s.Value
		}
	}
	if info.Version == "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	if info.Commit == "" && revision != "" {
		info.Commit = revision[:min(len(revision), shortCommit)]
		if modified == "true" {
			info.Commit += "-dirty"
		}
	}
	if info.Version == "" {
		info.Version = "devel"
	}
	return info
}
