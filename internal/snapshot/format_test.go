package snapshot

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

var aki2026 = domain.BashoID{Year: 2026, Month: time.September}

// recorded builds a tournament from the recorded sumo-api fixtures.
func recorded(t *testing.T, id domain.BashoID, days map[domain.Division][]int) Tournament {
	t.Helper()
	api := sumoapi.New(fixture.Getter{Dir: "../../testdata/sumoapi", Base: sumoapi.DefaultBaseURL}, sumoapi.DefaultBaseURL)
	ctx := t.Context()
	b, err := api.Basho(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	out := Tournament{Basho: b, Banzuke: map[domain.Division][]domain.BanzukeEntry{}}
	for div, ds := range days {
		if out.Banzuke[div], err = api.Banzuke(ctx, id, div); err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			bouts, err := api.Torikumi(ctx, id, div, d)
			if err != nil {
				t.Fatal(err)
			}
			out.Bouts = append(out.Bouts, bouts...)
		}
	}
	SortBouts(out.Bouts) // the order a decoded tournament comes back in
	return out
}

func TestTournamentRoundTrip(t *testing.T) {
	in := recorded(t, aki2026, map[domain.Division][]int{domain.DivMakuuchi: {1, 13, 15}, domain.DivJuryo: {1, 13, 15}})
	enc, err := EncodeTournament(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeTournament(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip changed the tournament:\n in: %+v\nout: %+v", in.Basho, out.Basho)
		for i := range min(len(in.Bouts), len(out.Bouts)) {
			if !reflect.DeepEqual(in.Bouts[i], out.Bouts[i]) {
				t.Fatalf("first differing bout:\n in: %+v\nout: %+v", in.Bouts[i], out.Bouts[i])
			}
		}
	}
	again, _ := EncodeTournament(out)
	if !bytes.Equal(enc, again) {
		t.Error("encoding is not deterministic: a rebuild would change git")
	}
}

func TestRikishiRoundTrip(t *testing.T) {
	in := []domain.Rikishi{
		{
			ID: 8850, NSKID: 4227, SumoDBID: 12451, Name: domain.Name{En: "Onosato", Jp: "大の里"}, Heya: "Nishonoseki",
			Shusshin: "Ishikawa", Birth: domain.NewDate(2000, time.June, 7), Debut: domain.BashoID{Year: 2023, Month: time.May}, Height: 192, Weight: 191,
		},
		{ID: 1, Name: domain.Name{En: "Old"}, Retired: true},
	}
	enc, err := EncodeRikishi(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeRikishi(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, []domain.Rikishi{in[1], in[0]}) {
		t.Errorf("round trip (sorted by id) = %+v", out)
	}
}

// TestTournamentSize checks the size budget (V8): about 400 tournaments of
// about 520 bouts and 70 banzuke entries must stay well under the 15 MB
// embedding limit. Only some days are recorded, so the records keep just
// those days' opponents, as a full tournament's records would match its bouts.
func TestTournamentSize(t *testing.T) {
	days := []int{1, 13, 15}
	in := recorded(t, aki2026, map[domain.Division][]int{domain.DivMakuuchi: days, domain.DivJuryo: days})
	for _, entries := range in.Banzuke {
		for i := range entries {
			for d := range domain.BashoDays {
				if !slices.Contains(days, d+1) {
					entries[i].Results[d].OpponentID, entries[i].Results[d].Kimarite = 0, ""
				}
			}
		}
	}
	enc, err := EncodeTournament(in)
	if err != nil {
		t.Fatal(err)
	}
	var f tournamentFile
	if err := decode(enc, &f); err != nil {
		t.Fatal(err)
	}
	for _, entries := range f.Banzuke {
		for _, e := range entries {
			if len(e.Except) > 0 {
				t.Errorf("wrestler %d: records disagree with bouts on %+v", e.ID, e.Except)
			}
		}
	}
	// Scale the recorded days to a full tournament: bouts ×15/3, entries once.
	entries := len(in.Banzuke[domain.DivMakuuchi]) + len(in.Banzuke[domain.DivJuryo])
	full := len(enc) * 15 / len(days) // an overestimate: entries are counted 5 times
	total := full * 400
	t.Logf("%d bouts, %d entries: %d bytes; a full tournament < %d B, 400 < %.1f MB", len(in.Bouts), entries, len(enc), full, float64(total)/1e6)
	if total > 10e6 {
		t.Errorf("estimated snapshot %.1f MB, budget 10 MB", float64(total)/1e6)
	}
}

// TestTournamentExceptions covers what deduplication must not lose: a record
// that disagrees with its bout, and a visitor from a lower division.
func TestTournamentExceptions(t *testing.T) {
	in := recorded(t, aki2026, map[domain.Division][]int{domain.DivJuryo: {13}})
	e := &in.Banzuke[domain.DivJuryo][0]
	e.Results[12].Kimarite = "differs"
	e.Results[0] = domain.DayResult{Outcome: domain.Absent}
	in.Bouts[0].East.Rank = domain.Rank{Title: domain.Makushita, Number: 2, Side: domain.East}
	in.Bouts[0].East.Shikona.En = "Visitor"
	enc, err := EncodeTournament(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeTournament(enc)
	if err != nil || !reflect.DeepEqual(in, out) {
		t.Errorf("round trip lost an exception: %v", err)
	}
}

func TestManifest(t *testing.T) {
	m := Manifest{
		Schema: Schema, Generator: "test", Source: "sumo-api.com",
		Basho: []domain.BashoID{domain.FirstBasho, aki2026}, Files: map[string]string{TournamentPath(aki2026): Checksum([]byte("x"))},
	}
	enc, err := EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"basho/202609.json.zst"`) {
		t.Errorf("manifest:\n%s", enc)
	}
	got, err := DecodeManifest(enc)
	if err != nil || !reflect.DeepEqual(got, m) {
		t.Fatalf("DecodeManifest = %+v, %v", got, err)
	}
	if !got.Has(aki2026) || got.Has(aki2026.Next()) || got.Newest() != aki2026 || (Manifest{}).Newest() != (domain.BashoID{}) {
		t.Error("Has/Newest")
	}
	if _, err := DecodeManifest(bytes.Replace(enc, []byte(`"schema": 1`), []byte(`"schema": 2`), 1)); err == nil {
		t.Error("a manifest of another schema was accepted")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := DecodeTournament([]byte("not zstd")); err == nil {
		t.Error("garbage decoded")
	}
	old, _ := encode(tournamentFile{versioned: versioned{Schema - 1}})
	if _, err := DecodeTournament(old); err == nil {
		t.Error("a file of another schema was accepted")
	}
	short, _ := encode(tournamentFile{versioned: versioned{Schema}, Banzuke: map[domain.Division][]entryFile{domain.DivJuryo: {{ID: 1, Record: "WL"}}}})
	if _, err := DecodeTournament(short); err == nil {
		t.Error("a short record was accepted")
	}
}
