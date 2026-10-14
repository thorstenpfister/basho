package cli

import (
	"strings"
	"sync"
	"testing"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/service"
)

// embeddedService is the binary's data without remote layers: any attempt
// to reach the network would fail the commands.
var embeddedService = sync.OnceValue(func() *service.Service {
	return service.New(service.Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
})

// The Phase 6 acceptance commands give correct, readable output from the
// embedded snapshot alone.
func TestAcceptanceOffline(t *testing.T) {
	deps := fixtureDeps(t)
	deps.Open = func(config.Config) (*service.Service, error) { return embeddedService(), nil }
	for _, tt := range []struct {
		args []string
		want []string
	}{
		{[]string{"day", "13", "-t", "202609"}, []string{
			"Aki 2026 · Makuuchi · Day 13 · Fri 2026-09-25 · Ryogoku Kokugikan",
			"Y1e        Onosato  11-1  ●  shitatenage  ○  6-6   Kirishima     O1e   結び",
			"K1e     Hakunofuji   3-9  □  fusen        ■  4-8   Takanosho     M4e",
			"Kyujo: Kotoshoho (M1e), Wakatakakage (M12e), Wakanosho (M16e), Hoshoryu (Y1w)",
		}},
		{[]string{"bout", "13", "19", "-t", "aki2026"}, []string{
			"大の里", "Nishonoseki", "11-1     RECORD     6-6", "10      H2H       1",
			"KIRISHIMA wins by SHITATENAGE  下手投げ (underarm throw)", "Onosato 11-2 · Kirishima 7-6",
		}},
		{[]string{"standings", "--day", "15", "-t", "2026-09", "--grid"}, []string{
			"Y1e   Onosato       ○ ○ ○ ● ○ ○ ○ ○ ○ ○ ○ ○ ● ● ○    12-3  KK",
			"Y1w   Hoshoryu      – – – – – – – – – – – – – – –  0-0-15  MK 休",
			"Yusho: Onosato",
		}},
	} {
		out, code := run(t, deps, tt.args...)
		if code != exitOK {
			t.Errorf("%v: exit %d\n%s", tt.args, code, out)
			continue
		}
		for _, w := range tt.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v: lacks %q:\n%s", tt.args, w, out)
			}
		}
	}
}
