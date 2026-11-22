package datatool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// The trimmed Kyushu 2026 recordings in testdata/live.
func TestAnalyzeRecordings(t *testing.T) {
	a, err := AnalyzeRecordings(t.Context(), "../../testdata/live")
	if err != nil {
		t.Fatal(err)
	}
	if a.Basho != (domain.BashoID{Year: 2026, Month: time.November}) || a.Interval != 10*time.Minute || len(a.Days) != 3 {
		t.Fatalf("basho %s, interval %s, %d days", a.Basho, a.Interval, len(a.Days))
	}
	at := func(d domain.Date, hm string) time.Time {
		h, _ := time.Parse("1504", hm)
		return d.Time().Add(time.Duration(h.Hour())*time.Hour + time.Duration(h.Minute())*time.Minute)
	}
	for i, want := range []struct {
		day, bouts, ticks   int
		osadaDone, sumoDone string
		firstStamp          string
	}{
		{1, 35, 6, "1800", "1800", "15:04"},
		{5, 34, 6, "1810", "1810", "15:04"},
		{15, 35, 8, "1740", "1730", "15:22"}, // 34 regular bouts and the playoff
	} {
		d := a.Days[i]
		if d.Day != want.day || len(d.Bouts) != want.bouts || len(d.Ticks) != want.ticks || d.Failed != 0 || len(d.ParseErrors) > 0 {
			t.Errorf("day %d: %d bouts, %d ticks, %d failed, %v", d.Day, len(d.Bouts), len(d.Ticks), d.Failed, d.ParseErrors)
		}
		if !d.OsadaComplete.Equal(at(d.Date, want.osadaDone)) || !d.SumoComplete.Equal(at(d.Date, want.sumoDone)) {
			t.Errorf("day %d complete: osada %s, sumo-api %s", d.Day, hhmm(d.OsadaComplete), hhmm(d.SumoComplete))
		}
		if hhmm(d.OsadaStamps[0]) != want.firstStamp {
			t.Errorf("day %d: first osada batch %s", d.Day, hhmm(d.OsadaStamps[0]))
		}
		for _, b := range d.Bouts {
			if b.Disagree || len(b.Changed) > 0 {
				t.Errorf("day %d %s %d: disagree %v, changed %v", d.Day, b.Division, b.MatchNo, b.Disagree, b.Changed)
			}
			if l, ok := b.Lead(); ok && l < 0 {
				t.Errorf("day %d %s %d: osada first by %s", d.Day, b.Division, b.MatchNo, -l)
			}
			if b.Playoff && (b.Division != domain.DivMakuuchi || b.Osada.IsZero() || !b.Sumo.IsZero() || hhmm(b.OsadaStamp) != "17:42") {
				t.Errorf("playoff: %+v", b)
			}
		}
	}
	var sb strings.Builder
	if err := a.WriteReport(&sb); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"**V1:** of 103 results", "| 15 | 2026-11-22 | 8 | 0 | 15:22 |", "so every request is a full answer", "Day 15 Makuuchi playoff 21: only osada lists it"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, sb.String())
		}
	}
}

// A result sumo-api corrects after showing it: a correction in sumo-api and,
// against osada's, a disagreement.
func TestAnalyzeCorrection(t *testing.T) {
	dir := t.TempDir()
	for _, tick := range []string{"1700", "1800"} {
		if err := os.CopyFS(filepath.Join(dir, "20261108", tick), os.DirFS(filepath.Join("../../testdata/live/20261108", tick))); err != nil {
			t.Fatal(err)
		}
	}
	name := filepath.Join(dir, "20261108/1800/sumoapi/basho_202611_torikumi_Makuuchi_1.json")
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	fixed := strings.Replace(string(raw), `"kimarite":"yorikiri"`, `"kimarite":"oshidashi"`, 1)
	if fixed == string(raw) {
		t.Fatal("no yorikiri to change")
	}
	if err := os.WriteFile(name, []byte(fixed), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := AnalyzeRecordings(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range a.Days[0].Bouts {
		want := b.Division == domain.DivMakuuchi && b.MatchNo == 1 && !b.Playoff
		if b.Disagree != want || (len(b.Changed) > 0) != want {
			t.Errorf("%s %d: disagree %v, changed %v", b.Division, b.MatchNo, b.Disagree, b.Changed)
		}
		if want && b.Changed[0] != "sumo-api" {
			t.Errorf("changed in %v", b.Changed)
		}
	}
}
