// Package archtest checks the repository against SPEC.md, so it cannot drift
// unnoticed: the dependency rules of §5.1 on the real import graph, and the
// Makefile targets of §14.1.
//
// The graph is loaded in-process with go/build rather than via `go list`: the
// go test cache only tracks files this process opens, so reading the sources
// here means any added or edited file invalidates a cached pass.
package archtest

import (
	"errors"
	"go/build"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	module = "github.com/thorstenpfister/basho"
	root   = "../.." // go test runs in the package directory
)

// pkg is a package by module-relative path ("internal/domain") with its
// direct, non-test imports, internal ones also module-relative.
type pkg struct {
	path    string
	imports []string
}

func under(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+"/")
}

// networkPkgs are the standard library packages that open connections.
var networkPkgs = []string{"net", "net/http"}

// uiAllowed is everything the TUI and CLI may import besides their own
// subtrees: data is reached only through internal/service. The live scheduler
// is wired into the TUI (SPEC §10.2); portraits are rendered there (§8.9).
// internal/derive is pure computation without I/O, and the service's views
// carry its types (records, profiles, standings).
var uiAllowed = []string{
	"internal/service", "internal/domain", "internal/derive", "internal/clock", "internal/config",
	"internal/spoiler", "internal/live", "internal/portrait", "internal/buildinfo",
}

// violations applies the SPEC §5.1 rules to a package graph.
func violations(pkgs []pkg) []string {
	var out []string
	for _, p := range pkgs {
		for _, imp := range p.imports {
			switch {
			case slices.Contains(networkPkgs, imp):
				if !under(p.path, "internal/source") && !under(p.path, "internal/httpx") {
					out = append(out, p.path+": imports "+imp+"; only internal/source/... and internal/httpx may touch the network")
				}
			case !under(imp, "internal") && !under(imp, "cmd") && imp != "data":
				// Standard library or third party: no rule. data (the embedded
				// snapshot) is ours and follows the rules below.
			case p.path == "internal/domain":
				out = append(out, p.path+": imports "+imp+"; domain imports nothing internal")
			case p.path == "internal/derive" && imp != "internal/domain":
				out = append(out, p.path+": imports "+imp+"; derive may import only internal/domain")
			case under(p.path, "internal/tui") || under(p.path, "internal/cli"):
				own := "internal/" + strings.Split(p.path, "/")[1]
				if !under(imp, own) && !slices.Contains(uiAllowed, imp) {
					out = append(out, p.path+": imports "+imp+"; the TUI and CLI reach data only through internal/service")
				}
			}
		}
	}
	if slices.Contains(reachable(pkgs, "cmd/basho"), "internal/datatool") {
		out = append(out, "cmd/basho: depends on internal/datatool; the developer tool is never shipped")
	}
	return out
}

// reachable returns every package transitively imported by from.
func reachable(pkgs []pkg, from string) []string {
	edges := map[string][]string{}
	for _, p := range pkgs {
		edges[p.path] = append(edges[p.path], p.imports...)
	}
	var seen []string
	stack := []string{from}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range edges[cur] {
			if !slices.Contains(seen, next) {
				seen = append(seen, next)
				stack = append(stack, next)
			}
		}
	}
	return seen
}

// load reads every package under cmd/ and internal/, skipping the directories
// the go tool ignores (testdata, and names starting with "." or "_").
func load(t *testing.T) []pkg {
	t.Helper()
	var pkgs []pkg
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(dir string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return err
			}
			if name := d.Name(); name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			bp, err := build.Default.ImportDir(dir, 0)
			if _, noGo := errors.AsType[*build.NoGoError](err); noGo {
				return nil
			}
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, dir)
			p := pkg{path: filepath.ToSlash(rel)}
			for _, imp := range bp.Imports {
				p.imports = append(p.imports, strings.TrimPrefix(imp, module+"/"))
			}
			pkgs = append(pkgs, p)
			return nil
		})
		if err != nil {
			t.Fatalf("load %s: %v", top, err)
		}
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages found")
	}
	return pkgs
}

func TestDependencyRules(t *testing.T) {
	for _, v := range violations(load(t)) {
		t.Error(v)
	}
}

func TestViolationsDetectsEachRule(t *testing.T) {
	graph := []pkg{
		{path: "internal/domain", imports: []string{"time", "internal/clock"}},
		{path: "internal/derive", imports: []string{"internal/domain", "internal/cache"}},
		{path: "internal/service", imports: []string{"net/http", "internal/datatool"}},
		{path: "internal/httpx", imports: []string{"net/http"}},
		{path: "internal/source/osada", imports: []string{"net/http", "internal/httpx"}},
		{path: "internal/tui/day", imports: []string{"internal/service", "internal/tui/theme", "internal/source/osada"}},
		{path: "internal/cli", imports: []string{"internal/domain", "internal/rating"}},
		{path: "internal/tui", imports: []string{"data"}},
		{path: "cmd/basho", imports: []string{"internal/service"}},
		{path: "cmd/basho-data", imports: []string{"internal/datatool"}},
	}
	got := violations(graph)
	want := []string{
		"internal/domain: imports internal/clock",
		"internal/derive: imports internal/cache",
		"internal/service: imports net/http",
		"internal/tui/day: imports internal/source/osada",
		"internal/cli: imports internal/rating",
		"internal/tui: imports data",
		"cmd/basho: depends on internal/datatool",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d violations, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i, w := range want {
		if !strings.HasPrefix(got[i], w) {
			t.Errorf("violation %d = %q, want prefix %q", i, got[i], w)
		}
	}
}
