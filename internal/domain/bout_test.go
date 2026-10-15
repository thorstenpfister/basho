package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOutcome(t *testing.T) {
	tests := []struct {
		o         Outcome
		name      string
		win, loss bool
	}{
		{Pending, "pending", false, false},
		{Win, "win", true, false},
		{Loss, "loss", false, true},
		{FusenWin, "fusen_win", true, false},
		{FusenLoss, "fusen_loss", false, true},
		{Absent, "absent", false, false},
		{Draw, "draw", false, false},
		{Unknown, "unknown", false, false},
	}
	for _, tt := range tests {
		if tt.o.String() != tt.name || tt.o.IsWin() != tt.win || tt.o.IsLoss() != tt.loss {
			t.Errorf("%d: %q win=%v loss=%v; want %q %v %v", tt.o, tt.o.String(), tt.o.IsWin(), tt.o.IsLoss(), tt.name, tt.win, tt.loss)
		}
	}
	if Outcome(99).String() != "unknown" {
		t.Error("out-of-range Outcome must print as unknown")
	}
	if b, _ := json.Marshal(DayResult{Outcome: FusenWin}); !strings.Contains(string(b), `"Outcome":"fusen_win"`) {
		t.Errorf("JSON = %s, want the outcome by name", b)
	}
}

func TestOutcomeText(t *testing.T) {
	for o := Pending; o <= Unknown; o++ {
		b, _ := o.MarshalText()
		var got Outcome
		if err := got.UnmarshalText(b); err != nil || got != o {
			t.Errorf("%v: round trip = %v, %v", o, got, err)
		}
	}
	var o Outcome
	if o.UnmarshalText([]byte("victory")) == nil {
		t.Error("unknown outcome accepted")
	}
}

func TestNameModes(t *testing.T) {
	n := Name{En: "Onosato", Jp: "大の里"}
	approx := Name{En: "Shonannomi", Approx: true}
	for _, tt := range []struct {
		mode                       NameMode
		show, alt, showNew, altNew string
	}{
		{NamesBoth, "Onosato", "大の里", "≈Shonannomi", ""},
		{NamesEn, "Onosato", "", "≈Shonannomi", ""},
		{NamesJp, "大の里", "Onosato", "≈Shonannomi", ""},
	} {
		if n.Show(tt.mode) != tt.show || n.Alt(tt.mode) != tt.alt || approx.Show(tt.mode) != tt.showNew || approx.Alt(tt.mode) != tt.altNew {
			t.Errorf("mode %d: %q %q %q %q", tt.mode, n.Show(tt.mode), n.Alt(tt.mode), approx.Show(tt.mode), approx.Alt(tt.mode))
		}
	}
	if ParseNameMode("jp") != NamesJp || ParseNameMode("en") != NamesEn || ParseNameMode("both") != NamesBoth {
		t.Error("ParseNameMode")
	}
}
