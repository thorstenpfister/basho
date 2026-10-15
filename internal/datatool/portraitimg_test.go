package datatool

import (
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
)

func TestPortraitOverrides(t *testing.T) {
	dir := t.TempDir()
	write := func(text string) string {
		p := filepath.Join(dir, "overrides.toml")
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	o, err := LoadPortraitOverrides(write(`
[[portrait]]
id = 7
crop = [10, 20, 110, 120]
note = "the referee's face was larger"

[[portrait]]
id = 8
exclude = true
`))
	if err != nil || len(o[7].Crop) != 4 || !o[8].Exclude {
		t.Errorf("%+v, %v", o, err)
	}
	if o, err := LoadPortraitOverrides(filepath.Join(dir, "missing.toml")); err != nil || len(o) != 0 {
		t.Errorf("a missing file: %v, %v", o, err)
	}
	for text, want := range map[string]string{
		"[[portrait]]\nid = 1\ncrop = [10, 10, 5, 50]\n": "crop wants",
		"[[portrait]]\nid = 1\ncolour = true\n":          "unknown setting",
		"[[portrait]]\nid = 1\n[[portrait]]\nid = 1\n":   "twice",
		"[[portrait]]\nid = 1\ncrop = [1, 2, 3]\n":       "crop wants",
	} {
		if _, err := LoadPortraitOverrides(write(text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", text, err, want)
		}
	}
}

func TestCropAround(t *testing.T) {
	b := image.Rect(0, 0, 500, 800)
	// A 100 px face in the middle: a 220 px square, the face 40% down.
	if got := cropAround(image.Rect(200, 300, 300, 400), b); got != image.Rect(140, 262, 360, 482) {
		t.Errorf("middle: %v", got)
	}
	// At the top left edge: moved inside.
	if got := cropAround(image.Rect(0, 0, 100, 100), b); got != image.Rect(0, 0, 220, 220) {
		t.Errorf("edge: %v", got)
	}
	// A face too large for the photo: the square shrinks to fit.
	if got := cropAround(image.Rect(0, 0, 400, 400), b); got.Dx() != 500 || got.Dy() != 500 {
		t.Errorf("large: %v", got)
	}
}

func TestAreaScale(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	for y := range 3 {
		for x := range 3 {
			img.SetNRGBA(x, y, color.NRGBA{uint8(90 * x), 0, 0, 255})
		}
	}
	// 3 px into 2: each output pixel covers one and a half columns.
	out := AreaScale(img, img.Bounds(), 2)
	if r0, r1 := out.NRGBAAt(0, 0).R, out.NRGBAAt(1, 0).R; r0 != 30 || r1 != 150 {
		t.Errorf("averages %d, %d; want 30, 150", r0, r1)
	}
}

func TestQuantize(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, len(Palette), 1))
	for i, c := range Palette {
		img.Set(i, 0, c)
	}
	q := Quantize(img, false)
	for i := range Palette {
		if int(q.ColorIndexAt(i, 0)) != i {
			t.Errorf("palette colour %d maps to %d", i, q.ColorIndexAt(i, 0))
		}
	}
	// A shaded skin tone stays a skin tone, not a grey or a blue.
	skin := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	skin.SetNRGBA(0, 0, color.NRGBA{0xb0, 0x80, 0x68, 255})
	if c := Palette[Quantize(skin, false).ColorIndexAt(0, 0)].(color.NRGBA); c.R <= c.B {
		t.Errorf("skin became %v", c)
	}
}

func TestMakePortrait(t *testing.T) {
	blank := image.NewNRGBA(image.Rect(0, 0, 300, 500))
	p := MakePortrait(blank, PortraitOverride{}, false)
	if p.Face || p.Crop != image.Rect(0, 0, 300, 300) {
		t.Errorf("no face: %+v", p.Crop)
	}
	if p.Big.Bounds().Dx() != BigPortrait || p.Small.Bounds().Dx() != SmallPortrait {
		t.Error("sizes")
	}
	p = MakePortrait(blank, PortraitOverride{Crop: []int{10, 20, 110, 120}}, true)
	if !p.Face || p.Crop != image.Rect(10, 20, 110, 120) {
		t.Errorf("override: %v", p.Crop)
	}
}

func TestFetchPortraitsOverrides(t *testing.T) {
	img := pngBytes(t)
	w := &wikimedia{sparql: sparqlAnswer, commons: commonsAnswer, thumbs: map[string][]byte{"https://thumb.wikimedia.org/h.png": img}}
	rs := []domain.Rikishi{{ID: 1, SumoDBID: 100, Name: domain.Name{En: "Onosato"}}, {ID: 7, Name: domain.Name{En: "Other"}}}
	ov := PortraitOverrides{1: {ID: 1, Exclude: true, Note: "a group photo"}, 7: {ID: 7, File: "Hoshoryu.png"}}
	set, err := FetchPortraits(t.Context(), w, rs, PortraitConfig{Work: t.TempDir(), Overrides: ov}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Sources) != 1 || set.Sources[0].RikishiID != 7 || set.Sources[0].File != "Hoshoryu.png" {
		t.Errorf("sources %+v", set.Sources)
	}
	if len(set.Skipped) != 1 || set.Skipped[0].Reason != "excluded in overrides.toml: a group photo" {
		t.Errorf("skipped %+v", set.Skipped)
	}
}

func TestBuildPortraits(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "images", "1.png"), pngBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}
	built, err := BuildPortraits(work, []PortraitSource{{RikishiID: 1, Name: "A", Image: filepath.Join("images", "1.png")}}, nil, false, io.Discard)
	if err != nil || len(built) != 1 {
		t.Fatal(built, err)
	}
	for _, f := range []string{"out/1-32.png", "out/1-20.png", "sheet.png"} {
		if _, err := os.Stat(filepath.Join(work, f)); err != nil {
			t.Error(err)
		}
	}
}

func TestWritePortraitData(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"32/99.png", "overrides.toml"} { // a portrait of an earlier run, and the overrides
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	blank := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	built := []BuiltPortrait{{
		PortraitSource: PortraitSource{
			RikishiID: 7, Name: "Onosato", File: "A|B.jpg", Author: "Photo | Co", Licence: "CC BY-SA 4.0",
			LicenceURL: "https://creativecommons.org/licenses/by-sa/4.0", SourceURL: "https://commons.wikimedia.org/wiki/File:A", Year: 2024,
		},
		Portrait: MakePortrait(blank, PortraitOverride{}, false),
	}}
	if err := WritePortraitData(dir, built); err != nil {
		t.Fatal(err)
	}
	set, err := portrait.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := set.Credit(7); !ok || c.Author != "Photo | Co" || c.Year != 2024 {
		t.Errorf("credit %+v", c)
	}
	for _, size := range portrait.Sizes {
		if _, err := set.Image(7, size); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "32", "99.png")); err == nil {
		t.Error("an earlier run's portrait stayed")
	}
	if _, err := os.Stat(filepath.Join(dir, "overrides.toml")); err != nil {
		t.Error("overrides.toml went")
	}
	md, _ := os.ReadFile(filepath.Join(dir, "CREDITS.md"))
	if !strings.Contains(string(md), `| Onosato | [A\|B.jpg](https://commons.wikimedia.org/wiki/File:A) | Photo \| Co | [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0) |`) {
		t.Errorf("CREDITS.md:\n%s", md)
	}
	if lic, _ := os.ReadFile(filepath.Join(dir, "LICENSE")); !strings.Contains(string(lic), "keeps the licence") {
		t.Errorf("LICENSE:\n%s", lic)
	}
}
