package fixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thorstenpfister/basho/internal/httpx"
)

func TestName(t *testing.T) {
	tests := map[string]string{
		"/api/basho/202609/torikumi/Makuuchi/15":     "basho_202609_torikumi_Makuuchi_15.json",
		"/api/rikishis?intai=true&limit=1000&skip=0": "rikishis_intai-true_limit-1000_skip-0.json",
		"/api/v1/torikumi.json":                      "torikumi.json",
	}
	for in, want := range tests {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "basho_202609.json"), []byte(`{"date":"202609"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g := Getter{Dir: dir, Base: "https://sumo.test"}
	resp, err := g.Get(t.Context(), "https://sumo.test/api/basho/202609", httpx.Validators{})
	if err != nil || !resp.OK() || string(resp.Body) != `{"date":"202609"}` {
		t.Errorf("recorded: %v, %v", resp, err)
	}
	if resp, err := g.Get(t.Context(), "https://sumo.test/api/basho/202611", httpx.Validators{}); err != nil || resp.Status != 404 {
		t.Errorf("not recorded: %v, %v; want 404", resp, err)
	}
	if _, err := g.Get(t.Context(), "https://elsewhere.test/api/basho/202609", httpx.Validators{}); err == nil {
		t.Error("a URL outside Base must be an error")
	}
}
