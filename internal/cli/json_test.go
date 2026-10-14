package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

func TestJSONEnvelopes(t *testing.T) {
	deps := fixtureDeps(t)
	for _, tt := range []struct {
		args []string
		kind string
		data func(map[string]any) bool
	}{
		{[]string{"day", "13", "-t", "202609"}, "day", func(d map[string]any) bool {
			bouts := d["bouts"].([]any)
			last := bouts[len(bouts)-1].(map[string]any)
			return d["division"] == "makuuchi" && d["status"] == "complete" && last["winner"] == "west" && last["musubi"] == true
		}},
		{[]string{"bout", "13", "19", "-t", "aki2026"}, "bout", func(d map[string]any) bool {
			east := d["east"].(map[string]any)
			rank := east["rank"].(map[string]any)
			return rank["title"] == "yokozuna" && rank["short"] == "Y1e" && d["result"].(map[string]any)["winner"] == "west"
		}},
		{[]string{"standings", "--day", "15", "-t", "202609"}, "standings", func(d map[string]any) bool {
			row := d["rows"].([]any)[0].(map[string]any)
			return d["yushoDecided"] == true && row["band"] == 0.0 && len(row["results"].([]any)) == 15
		}},
		{[]string{"info", "202005"}, "info", func(d map[string]any) bool { return d["status"] == "cancelled" && d["start"] == nil }},
		{[]string{"list", "--from", "2026"}, "list", nil},
		{[]string{"ratings", "-t", "202609", "--top", "2"}, "ratings", func(d map[string]any) bool { return len(d["rows"].([]any)) == 2 }},
		{[]string{"credits"}, "credits", nil},
	} {
		out, code := run(t, deps, append(tt.args, "--json")...)
		if code != exitOK {
			t.Errorf("%v: exit %d", tt.args, code)
			continue
		}
		var env struct {
			SchemaVersion int            `json:"schemaVersion"`
			Kind          string         `json:"kind"`
			Provenance    map[string]any `json:"provenance"`
			Data          any            `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Errorf("%v: %v\n%s", tt.args, err, out)
			continue
		}
		if env.SchemaVersion != 1 || env.Kind != tt.kind || env.Provenance["source"] == nil {
			t.Errorf("%v: envelope %d %q %v", tt.args, env.SchemaVersion, env.Kind, env.Provenance)
		}
		if d, ok := env.Data.(map[string]any); tt.data != nil && (!ok || !tt.data(d)) {
			t.Errorf("%v: data %s", tt.args, out)
		}
	}
}

// failing is a remote that always fails like an unreachable sumo-api.
type failing struct{ err error }

func (f failing) Basho(context.Context, domain.BashoID) (domain.Basho, error) {
	return domain.Basho{}, f.err
}

func (f failing) Banzuke(context.Context, domain.BashoID, domain.Division) ([]domain.BanzukeEntry, error) {
	return nil, f.err
}

func (f failing) Torikumi(context.Context, domain.BashoID, domain.Division, int) ([]domain.Bout, error) {
	return nil, f.err
}

func TestExitCodes(t *testing.T) {
	withRemote := func(r service.TournamentSource) Deps {
		d := fixtureDeps(t)
		d.Open = func(config.Config) (*service.Service, error) {
			return service.New(service.Options{
				Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../testdata/snapshot")) },
				Params:   func() (rating.File, error) { return rating.File{}, nil },
				Remote:   r, Clock: clock.NewFake(testNow),
			}), nil
		}
		return d
	}
	for _, tt := range []struct {
		name string
		deps Deps
		args []string
		want int
	}{
		{"a bout out of range", fixtureDeps(t), []string{"bout", "13", "99", "-t", "202609"}, exitNotFound},
		{"day 16", fixtureDeps(t), []string{"day", "16", "-t", "202609"}, exitNotFound},
		{"not published yet", withRemote(failing{fmt.Errorf("x: %w", source.ErrNotYetAvailable)}), []string{"day", "1", "-t", "202611"}, exitNotFound},
		{"every source failed", withRemote(failing{fmt.Errorf("x: %w", source.ErrUpstream)}), []string{"day", "1", "-t", "202611"}, exitUnavailable},
		{"rate limited", withRemote(failing{fmt.Errorf("x: %w", source.ErrRateLimited)}), []string{"info", "202611"}, exitUnavailable},
	} {
		if _, code := run(t, tt.deps, tt.args...); code != tt.want {
			t.Errorf("%s: exit %d, want %d", tt.name, code, tt.want)
		}
	}
}

func TestRound(t *testing.T) {
	if roundTo(0.123456, 4) != 0.1235 || roundTo(-0.00005, 4) != -0.0001 || round(nil) != nil {
		t.Error("rounding")
	}
}
