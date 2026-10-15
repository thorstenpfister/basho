package portrait

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
)

func paletted(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, size, size), color.Palette{color.Black, color.White})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSet(t *testing.T) {
	fsys := fstest.MapFS{
		"credits.json": {Data: []byte(`[{"id":2,"name":"Onosato","file":"O.jpg","author":"A","licence":"CC BY-SA 4.0","sourceUrl":"u","year":2024},
			{"id":1,"name":"Hoshoryu","file":"H.jpg","author":"B","licence":"CC0","sourceUrl":"v"}]`)},
		"48/2.png": {Data: paletted(t, 48)},
		"32/2.png": {Data: paletted(t, 32)},
		"20/2.png": {Data: paletted(t, 20)},
		"32/1.png": {Data: []byte("not a png")},
	}
	s, err := Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if c := s.Credits(); len(c) != 2 || c[0].Name != "Hoshoryu" {
		t.Errorf("credits by name: %+v", c)
	}
	if c, ok := s.Credit(2); !ok || c.Year != 2024 {
		t.Errorf("credit %+v", c)
	}
	for _, size := range Sizes {
		if img, err := s.Image(2, size); err != nil || img.Bounds().Dx() != size {
			t.Errorf("Onosato at %d: %v", size, err)
		}
	}
	if _, err := s.Image(1, 32); err == nil {
		t.Error("a broken file decoded")
	}
	if _, err := s.Image(3, 32); err == nil {
		t.Error("a portrait without credit")
	}
	if Path(2, 32) != "32/2.png" {
		t.Error(Path(2, 32))
	}
	if len(Empty().Credits()) != 0 || Empty().Credits() == nil {
		t.Error("Empty")
	}
}
