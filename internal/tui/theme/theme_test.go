package theme

import (
	"reflect"
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestGlyphSetsComplete(t *testing.T) {
	for name, g := range map[string]Glyphs{"unicode": Unicode, "ascii": ASCII} {
		v := reflect.ValueOf(g)
		for i := range v.NumField() {
			if v.Field(i).String() == "" {
				t.Errorf("%s: %s is empty", name, v.Type().Field(i).Name)
			}
		}
	}
	for i, r := range ASCII.Win + ASCII.Loss + ASCII.Spoiler + ASCII.Musubi {
		if r > 127 {
			t.Errorf("ASCII glyph %d is %q", i, r)
		}
	}
}

func TestOutcome(t *testing.T) {
	for o, want := range map[domain.Outcome]string{
		domain.Win: "○", domain.Loss: "●", domain.FusenWin: "□", domain.FusenLoss: "■",
		domain.Absent: "–", domain.Pending: "·", domain.Draw: "△", domain.Unknown: "?",
	} {
		if got := Unicode.Outcome(o); got != want {
			t.Errorf("%v: %q, want %q", o, got, want)
		}
	}
}

func TestNew(t *testing.T) {
	dark, light := New(true, false), New(false, true)
	if dark.Colors.Fg == light.Colors.Fg || dark.Colors.Accent == light.Colors.Accent {
		t.Error("the palette does not adapt to the background")
	}
	if light.Win != "o" || dark.Win != "○" {
		t.Error("glyph sets")
	}
	if got := dark.Record("8-7", true, false); got == "8-7" {
		t.Error("kachi-koshi not coloured")
	}
	if got := dark.Record("7-7", false, false); got != "7-7" {
		t.Error("a plain record coloured")
	}
}
