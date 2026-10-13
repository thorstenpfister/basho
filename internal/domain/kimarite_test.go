package domain

import (
	"testing"
)

// sumoAPIKimarite is every value sumo-api.com's /kimarite endpoint returned on
// 2026-10-13 (V11).
var sumoAPIKimarite = []Kimarite{
	"abisetaoshi", "amiuchi", "ashitori", "chongake", "dashinage", "fumidashi", "fusen", "gasshohineri",
	"hansoku", "harimanage", "hatakikomi", "hikiotoshi", "hikiwake", "hikkake", "ipponzeoi", "isamiashi",
	"itamiwake", "izori", "kainahineri", "kakenage", "katasukashi", "kawazugake", "kekaeshi", "ketaguri",
	"kimedashi", "kimetaoshi", "kirikaeshi", "komatasukui", "koshikudake", "koshinage", "kotehineri",
	"kotenage", "kozumatori", "kubihineri", "kubinage", "makiotoshi", "mitokorozeme", "nichonage",
	"nimaigeri", "okuridashi", "okurigake", "okurihikiotoshi", "okurinage", "okuritaoshi",
	"okuritsuridashi", "okuritsuriotoshi", "omata", "osakate", "oshidashi", "oshitaoshi", "sabaori",
	"sakatottari", "shitatedashinage", "shitatehineri", "shitatenage", "sokubiotoshi", "sotogake",
	"sotokomata", "sotomuso", "sukuinage", "susoharai", "susotori", "tasukizori", "tokkurinage", "tottari",
	"tsukaminage", "tsukidashi", "tsukihiza", "tsukiotoshi", "tsukitaoshi", "tsukite", "tsumatori",
	"tsuridashi", "tsuriotoshi", "tsutaezori", "uchigake", "uchimuso", "ushiromotare", "utchari",
	"uwatedashinage", "uwatehineri", "uwatenage", "waridashi", "watashikomi", "yaguranage", "yobikaeshi",
	"yobimodoshi", "yorikiri", "yoritaoshi", "zubuneri",
}

func TestKimariteTableCoversSumoAPI(t *testing.T) {
	if len(sumoAPIKimarite) != 90 {
		t.Fatalf("fixture has %d values, want 90", len(sumoAPIKimarite))
	}
	for _, k := range sumoAPIKimarite {
		if _, ok := k.Info(); !ok {
			t.Errorf("sumo-api value %q missing from the table", k)
		}
	}
}

func TestKimariteTableShape(t *testing.T) {
	// Not one of the 82 official techniques: non-techniques (hiwaza),
	// disqualification, default, draws, and two historical spellings.
	notOfficial := []Kimarite{
		"isamiashi", "koshikudake", "tsukite", "tsukihiza", "fumidashi",
		"hansoku", "fusen", "hikiwake", "itamiwake", "dashinage", "yobikaeshi",
	}
	if official := len(kimariteTable) - len(notOfficial); official != 82 {
		t.Errorf("%d official techniques, want 82", official)
	}
	if len(kimariteByKanji) != len(kimariteTable) {
		t.Errorf("%d kanji for %d techniques: kanji must be unique for the osada.us lookup", len(kimariteByKanji), len(kimariteTable))
	}
	for k, info := range kimariteTable {
		if info.Kanji == "" || info.English == "" || info.Category == 0 {
			t.Errorf("%s: incomplete entry %+v", k, info)
		}
		if NormalizeKimarite(string(k)) != k {
			t.Errorf("%s is not in canonical form", k)
		}
	}
}

func TestKimariteCategories(t *testing.T) {
	tests := map[Kimarite]KimariteCategory{
		"oshidashi":   Push,
		"yorikiri":    Belt,
		"uwatenage":   Throw,
		"tsukiotoshi": Throw,
		"hatakikomi":  Pull,
		"isamiashi":   Other,
		"fusen":       NoTechnique,
		"hikiwake":    NoTechnique,
		"mystery":     Other, // unknown keys count as Other
	}
	for k, want := range tests {
		if got := k.Category(); got != want {
			t.Errorf("%s: Category = %s, want %s", k, got, want)
		}
	}
}

func TestKimariteLookups(t *testing.T) {
	if got := NormalizeKimarite(" Uwate-Dashinage "); got != "uwatedashinage" {
		t.Errorf("NormalizeKimarite = %q", got)
	}
	for in, want := range map[string]Kimarite{"押し出し": "oshidashi", "寄り切り": "yorikiri", "不戦": "fusen"} {
		if got, ok := KimariteByKanji(in); !ok || got != want {
			t.Errorf("KimariteByKanji(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := KimariteByKanji("未定"); ok {
		t.Error("未定 (undecided) must not map to a technique")
	}
}
