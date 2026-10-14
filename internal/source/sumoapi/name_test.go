package sumoapi

import "testing"

func TestCleanShikona(t *testing.T) {
	tests := map[string]string{
		"大の里　泰輝":  "大の里", // sumo-api: ring name, full-width space, given name
		"伊波(いなみ)": "伊波",  // bracketed reading
		"伊波（いなみ）": "伊波",  // full-width brackets
		"大の里":     "大の里", // already clean
		" 大栄翔 ":   "大栄翔",
		"":        "", // old records have no kanji
		"　泰輝":     "", // nothing before the space
	}
	for in, want := range tests {
		if got := cleanShikona(in); got != want {
			t.Errorf("cleanShikona(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanShikonaEn(t *testing.T) {
	for in, want := range map[string]string{
		"Hakuho Sho": "Hakuho", // retired: ring name and given name
		"Onosato":    "Onosato",
		" Abi ":      "Abi",
		"":           "",
	} {
		if got := cleanShikonaEn(in); got != want {
			t.Errorf("cleanShikonaEn(%q) = %q, want %q", in, got, want)
		}
	}
}
