package archtest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMakefileMatchesSpec keeps the Makefile and the target table of SPEC
// §14.1 in sync: the same targets in both, and a "(phase N)" in a target's
// description (a placeholder) must match the table's phase column.
func TestMakefileMatchesSpec(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile(filepath.Join(root, "SPEC.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(spec), "### 14.1 Makefile")
	if !ok {
		t.Fatal("SPEC.md has no §14.1 Makefile section")
	}
	section, _, _ = strings.Cut(section, "\n### ")

	// target → description, from "name: [deps] ## description".
	got := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^([a-z][a-z-]*):.*## (.*)$`).FindAllStringSubmatch(string(makefile), -1) {
		got[m[1]] = m[2]
	}
	// target → phase, from "| `name` | what it does | phase |".
	want := map[string]string{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z][a-z-]*)` \\|.*\\| (\\d+) \\|$").FindAllStringSubmatch(section, -1) {
		want[m[1]] = m[2]
	}
	if len(want) == 0 {
		t.Fatal("no targets found in SPEC §14.1")
	}

	phaseRe := regexp.MustCompile(`\(phase (\d+)\)`)
	for name, phase := range want {
		desc, ok := got[name]
		if !ok {
			t.Errorf("SPEC §14.1 lists %q, but the Makefile has no documented target of that name", name)
			continue
		}
		if m := phaseRe.FindStringSubmatch(desc); m != nil && m[1] != phase {
			t.Errorf("Makefile %q says phase %s, SPEC §14.1 says phase %s", name, m[1], phase)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("Makefile has target %q, which SPEC §14.1 does not list", name)
		}
	}
}
