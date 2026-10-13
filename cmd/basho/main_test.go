package main

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionViaLdflags builds the real binary the way the Makefile and
// GoReleaser do and checks that the injected values reach the output. The
// exact format is covered by internal/cli.
func TestVersionViaLdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	trackSources(t)
	bin := filepath.Join(t.TempDir(), "basho")
	const pkg = "github.com/thorstenpfister/basho/internal/buildinfo"
	ldflags := "-X " + pkg + ".Version=v9.8.7 -X " + pkg + ".Commit=abcdef123456 -X " + pkg + ".Date=2026-10-13T12:00:00Z"
	if out, err := exec.Command("go", "build", "-ldflags", ldflags, "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	for _, arg := range []string{"version", "--version"} {
		out, err := exec.Command(bin, arg).CombinedOutput()
		if err != nil {
			t.Fatalf("basho %s: %v\n%s", arg, err, out)
		}
		for _, want := range []string{"v9.8.7", "abcdef123456", "2026-10-13T12:00:00Z"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("basho %s = %q, missing %q", arg, out, want)
			}
		}
	}
}

// trackSources makes the go test cache depend on the sources the `go build`
// subprocess compiles. The test binary itself does not change with them: its
// entry point is the test runner, so main() and everything only it reaches
// are dropped by the linker. Opening each directory records its listing
// (names, sizes, mtimes), so any edit invalidates a cached pass.
func trackSources(t *testing.T) {
	t.Helper()
	for _, dir := range []string{".", "../../internal"} {
		err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return err
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}
