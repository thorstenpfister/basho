package cli

import (
	"strings"
	"testing"
)

// SPEC §9: --reveal-to sets the horizon, --no-spoilers applies the stored one.
func TestSpoilerFlags(t *testing.T) {
	deps := fixtureDeps(t)
	out, code := run(t, deps, "day", "13", "-t", "202609", "--reveal-to", "13:5")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	lines := strings.Split(out, "\n")
	shown, hidden := 0, 0
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		switch {
		case strings.Contains(l, " ? "):
			hidden++
		case strings.Contains(l, "○") || strings.Contains(l, "●"):
			shown++
		}
	}
	if shown != 5 || hidden != 14 {
		t.Errorf("%d shown, %d hidden:\n%s", shown, hidden, out)
	}

	// The horizon is stored: --no-spoilers in a later run applies it.
	out, _ = run(t, deps, "bout", "13", "19", "-t", "202609", "--no-spoilers")
	if !strings.Contains(out, "Result hidden (spoiler-free)") || strings.Contains(out, "wins by") {
		t.Errorf("bout 19:\n%s", out)
	}
	out, _ = run(t, deps, "bout", "13", "5", "-t", "202609", "--no-spoilers")
	if !strings.Contains(out, "wins by") {
		t.Errorf("bout 5:\n%s", out)
	}
	// Without the flag everything shows.
	if out, _ = run(t, deps, "bout", "13", "19", "-t", "202609"); !strings.Contains(out, "KIRISHIMA wins by SHITATENAGE") {
		t.Errorf("without --no-spoilers:\n%s", out)
	}
	// Standings and info keep the yusho to themselves.
	out, _ = run(t, deps, "standings", "--day", "15", "-t", "202609", "--no-spoilers")
	if strings.Contains(out, "Yusho: Onosato") || !strings.Contains(out, "11-1") {
		t.Errorf("standings:\n%s", out)
	}
	out, _ = run(t, deps, "info", "202609", "--no-spoilers")
	if strings.Contains(out, "Onosato") {
		t.Errorf("info:\n%s", out)
	}
	// JSON marks hidden bouts.
	out, _ = run(t, deps, "day", "13", "-t", "202609", "--no-spoilers", "--json")
	if strings.Count(out, `"hidden": true`)+strings.Count(out, `"hidden":true`) != 14 {
		t.Errorf("json: %d hidden", strings.Count(out, `"hidden"`))
	}
	// The whole day, then nothing.
	out, _ = run(t, deps, "day", "13", "-t", "202609", "--reveal-to", "13")
	if strings.Contains(out, " ? ") {
		t.Errorf("--reveal-to 13 left bouts hidden:\n%s", out)
	}
	out, _ = run(t, deps, "day", "1", "-t", "202609", "--reveal-to", "0")
	if !strings.Contains(out, " ? ") {
		t.Errorf("--reveal-to 0 showed results:\n%s", out)
	}
}

func TestParseHorizon(t *testing.T) {
	for _, in := range []string{"13", "13:19", "j13:5", "0", "15:21"} {
		if _, err := parseHorizon(in); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
	for _, in := range []string{"", "16", "x", "13:0", "0:3", "13:x", "-1"} {
		if _, err := parseHorizon(in); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
	if _, code := run(t, fixtureDeps(t), "day", "13", "--reveal-to", "16"); code != 2 {
		t.Errorf("exit %d for a bad --reveal-to", code)
	}
}
