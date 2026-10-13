package sumoapi

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestFreshnessPolicy(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile("../../../testdata/sumoapi/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	jst := func(m time.Month, d, h, min int) time.Time { return time.Date(2026, m, d, h, min, 0, 0, domain.JST) }
	const base = DefaultBaseURL + "/api"
	// Aki 2026 ran 2026-09-13 to 09-27. The recordings were made after it
	// ended; without its yusho, day 13 stands in for a tournament in progress.
	aki := read("basho_202609.json")
	var day13 map[string]any
	if err := json.Unmarshal(read("basho_202609_torikumi_Makuuchi_13.json"), &day13); err != nil {
		t.Fatal(err)
	}
	delete(day13, "yusho")
	akiDay13, _ := json.Marshal(day13)
	f := Freshness{LiveInterval: 2 * time.Minute}

	for _, c := range []struct {
		name       string
		url        string
		status     int
		body       []byte
		now        time.Time
		ttl        time.Duration
		wantPinned bool
	}{
		{"finished tournament", base + "/basho/202609", 200, aki, jst(10, 13, 12, 0), 0, true},
		{"finished bout list", base + "/basho/202609/torikumi/Makuuchi/15", 200, read("basho_202609_torikumi_Makuuchi_15.json"), jst(10, 13, 12, 0), 0, true},
		{"finished banzuke", base + "/basho/202609/banzuke/Makuuchi", 200, read("basho_202609_banzuke_Makuuchi.json"), jst(10, 13, 12, 0), 0, true},
		{"within the 2-day margin", base + "/basho/202609", 200, aki, jst(9, 28, 12, 0), time.Hour, false},
		{"banzuke within the margin", base + "/basho/202609/banzuke/Makuuchi", 200, nil, jst(9, 28, 12, 0), time.Hour, false},
		{"no yusho yet, long after", base + "/basho/202609/torikumi/Makuuchi/13", 200, akiDay13, jst(10, 13, 12, 0), 30 * time.Minute, false},
		{"an earlier day", base + "/basho/202609/torikumi/Makuuchi/13", 200, akiDay13, jst(9, 26, 15, 0), 30 * time.Minute, false},
		{"today, in the window", base + "/basho/202609/torikumi/Makuuchi/13", 200, akiDay13, jst(9, 25, 15, 0), 2 * time.Minute, false},
		{"today, before the window", base + "/basho/202609/torikumi/Makuuchi/13", 200, akiDay13, jst(9, 25, 9, 0), time.Hour, false},
		{"tomorrow's bouts", base + "/basho/202609/torikumi/Makuuchi/14", 200, akiDay13, jst(9, 25, 15, 0), 2 * time.Minute, false},
		{"unknown tournament", base + "/basho/202611", 200, read("basho_202611.json"), jst(10, 13, 12, 0), 6 * time.Hour, false},
		{"not found", base + "/basho/203001", http.StatusNotFound, nil, jst(10, 13, 12, 0), 6 * time.Hour, false},
		{"wrestler index", base + "/rikishis?intai=true&limit=1000&skip=0", 200, nil, jst(10, 13, 12, 0), 7 * 24 * time.Hour, false},
	} {
		ttl, pinned := f.Policy(c.url, c.status, c.body, c.now)
		if pinned != c.wantPinned || (!pinned && ttl != c.ttl) {
			t.Errorf("%s: ttl %v pinned %v, want %v %v", c.name, ttl, pinned, c.ttl, c.wantPinned)
		}
	}

	// Day 15 keeps the live interval until 19:15 for a playoff.
	if ttl, _ := (Freshness{}).Policy(base+"/basho/202609/torikumi/Makuuchi/15", 200, akiDay13, jst(9, 27, 19, 0)); ttl != 3*time.Minute {
		t.Errorf("day 15 at 19:00: ttl %v, want the default live interval", ttl)
	}
}
