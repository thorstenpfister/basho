package datatool

import (
	"bytes"
	_ "embed" // the face cascade
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	pigo "github.com/esimov/pigo/core"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/thorstenpfister/basho/internal/atomicfile"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
)

// cropFactor is the crop's side in face widths: head and a little of the
// shoulders. 2.2 gave the face too few of the pixels.
const cropFactor = 1.7

// faceTop is where the face's centre sits in the crop, from the top.
const faceTop = 0.45

// DefaultColours is each portrait's palette size: few enough for the pixel
// look, enough that faces keep their shading.
const DefaultColours = 16

//go:embed cascade/facefinder
var facefinder []byte

// PortraitOverride is a manual fix from data/portraits/overrides.toml.
type PortraitOverride struct {
	ID      domain.RikishiID `toml:"id"`
	Crop    []int            `toml:"crop"`    // x0, y0, x1, y1 in the thumbnail's pixels
	Exclude bool             `toml:"exclude"` // no portrait for this wrestler
	File    string           `toml:"file"`    // another Commons file than Wikidata's
	Note    string           `toml:"note"`    // why
}

// PortraitOverrides are the fixes by wrestler.
type PortraitOverrides map[domain.RikishiID]PortraitOverride

// LoadPortraitOverrides reads overrides.toml; a missing file has none.
func LoadPortraitOverrides(path string) (PortraitOverrides, error) {
	var f struct {
		Portrait []PortraitOverride `toml:"portrait"`
	}
	md, err := toml.DecodeFile(path, &f)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return PortraitOverrides{}, nil
	case err != nil:
		return nil, fmt.Errorf("%s: %w", path, err)
	case len(md.Undecoded()) > 0:
		return nil, fmt.Errorf("%s: unknown setting %s", path, md.Undecoded()[0])
	}
	out := PortraitOverrides{}
	for _, o := range f.Portrait {
		if o.Crop != nil && (len(o.Crop) != 4 || o.Crop[0] >= o.Crop[2] || o.Crop[1] >= o.Crop[3]) {
			return nil, fmt.Errorf("%s: wrestler %d: crop wants x0, y0, x1, y1 with x0 < x1 and y0 < y1", path, o.ID)
		}
		if _, dup := out[o.ID]; dup {
			return nil, fmt.Errorf("%s: wrestler %d twice", path, o.ID)
		}
		out[o.ID] = o
	}
	return out, nil
}

// Portrait is one wrestler's pixel portraits.
type Portrait struct {
	Images map[int]*image.Paletted // by size (portrait.Sizes), sharing one palette
	Crop   image.Rectangle         // in the thumbnail
	Face   bool                    // the crop follows a detected face, or an override
}

// ImageConfig is how the portraits are reduced.
type ImageConfig struct {
	Colours int  // palette size per portrait; zero: DefaultColours
	Dither  bool // ordered dithering
}

// MakePortrait crops img to head and shoulders, around the face found or
// the override's box, and reduces it to every size with a palette of its
// own colours, picked from the largest size, so a wrestler looks alike at
// all of them.
func MakePortrait(img image.Image, o PortraitOverride, cfg ImageConfig) Portrait {
	b := img.Bounds()
	var p Portrait
	switch face, ok := findFace(img); {
	case len(o.Crop) == 4:
		p.Crop, p.Face = image.Rect(o.Crop[0], o.Crop[1], o.Crop[2], o.Crop[3]).Add(b.Min).Intersect(b), true
	case ok:
		p.Crop, p.Face = cropAround(face, b), true
	default: // a square from the top, where the head usually is
		side := min(b.Dx(), b.Dy())
		x := b.Min.X + (b.Dx()-side)/2
		p.Crop = image.Rect(x, b.Min.Y, x+side, b.Min.Y+side)
	}
	colours := cfg.Colours
	if colours == 0 {
		colours = DefaultColours
	}
	pal := MedianCut(AreaScale(img, p.Crop, portrait.Sizes[0]), colours)
	p.Images = map[int]*image.Paletted{}
	for _, size := range portrait.Sizes {
		p.Images[size] = Quantize(AreaScale(img, p.Crop, size), pal, cfg.Dither)
	}
	return p
}

// findFace returns the largest face pigo finds with confidence.
func findFace(img image.Image) (image.Rectangle, bool) {
	classifier, err := pigo.NewPigo().Unpack(facefinder)
	if err != nil {
		panic(err) // the embedded cascade
	}
	b := img.Bounds()
	gray := make([]uint8, b.Dx()*b.Dy())
	for y := range b.Dy() {
		for x := range b.Dx() {
			gray[y*b.Dx()+x] = color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y
		}
	}
	dets := classifier.RunCascade(pigo.CascadeParams{
		MinSize: 20, MaxSize: max(b.Dx(), b.Dy()), ShiftFactor: 0.1, ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{Pixels: gray, Rows: b.Dy(), Cols: b.Dx(), Dim: b.Dx()},
	}, 0)
	dets = classifier.ClusterDetections(dets, 0.2)
	dets = slices.DeleteFunc(dets, func(d pigo.Detection) bool { return d.Q < 5 }) // pigo's usual threshold
	if len(dets) == 0 {
		return image.Rectangle{}, false
	}
	d := slices.MaxFunc(dets, func(a, b pigo.Detection) int {
		if a.Scale != b.Scale {
			return a.Scale - b.Scale
		}
		return int(math.Copysign(1, float64(a.Q-b.Q)))
	})
	half := d.Scale / 2
	return image.Rect(d.Col-half, d.Row-half, d.Col+half, d.Row+half).Add(b.Min), true
}

// cropAround is a square of cropFactor face widths with the face's centre
// faceTop from its top, moved and if need be shrunk to stay inside b.
func cropAround(face, b image.Rectangle) image.Rectangle {
	side := min(int(cropFactor*float64(face.Dx())), b.Dx(), b.Dy())
	cx, cy := (face.Min.X+face.Max.X)/2, (face.Min.Y+face.Max.Y)/2
	x := min(max(cx-side/2, b.Min.X), b.Max.X-side)
	y := min(max(cy-int(faceTop*float64(side)), b.Min.Y), b.Max.Y-side)
	return image.Rect(x, y, x+side, y+side)
}

// AreaScale scales the rectangle r of img to n×n pixels, each the average
// of the area it covers, partly covered pixels by their share.
func AreaScale(img image.Image, r image.Rectangle, n int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	sx, sy := float64(r.Dx())/float64(n), float64(r.Dy())/float64(n)
	for oy := range n {
		y0, y1 := float64(r.Min.Y)+float64(oy)*sy, float64(r.Min.Y)+float64(oy+1)*sy
		for ox := range n {
			x0, x1 := float64(r.Min.X)+float64(ox)*sx, float64(r.Min.X)+float64(ox+1)*sx
			var sum [3]float64
			var area float64
			for y := int(y0); float64(y) < y1; y++ {
				wy := math.Min(y1, float64(y+1)) - math.Max(y0, float64(y))
				for x := int(x0); float64(x) < x1; x++ {
					w := wy * (math.Min(x1, float64(x+1)) - math.Max(x0, float64(x)))
					c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					sum[0] += w * float64(c.R)
					sum[1] += w * float64(c.G)
					sum[2] += w * float64(c.B)
					area += w
				}
			}
			out.SetNRGBA(ox, oy, color.NRGBA{uint8(sum[0]/area + 0.5), uint8(sum[1]/area + 0.5), uint8(sum[2]/area + 0.5), 255})
		}
	}
	return out
}

// MedianCut picks a palette of up to n colours from img (SPEC §8.9): the
// pixels in CIELAB form one box, and the box with the widest range is cut
// at its median along that range, until there are n; each colour is its
// box's mean. Stable sorting makes it the same on every run.
func MedianCut(img *image.NRGBA, n int) color.Palette {
	type px struct {
		lab [3]float64
		c   color.NRGBA
	}
	b := img.Bounds()
	all := make([]px, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			l, a, bb := lab(float64(c.R), float64(c.G), float64(c.B))
			all = append(all, px{[3]float64{l, a, bb}, c})
		}
	}
	widest := func(box []px) (axis int, width float64) {
		for d := range 3 {
			lo, hi := box[0].lab[d], box[0].lab[d]
			for _, p := range box {
				lo, hi = min(lo, p.lab[d]), max(hi, p.lab[d])
			}
			if hi-lo > width {
				axis, width = d, hi-lo
			}
		}
		return axis, width
	}
	boxes := [][]px{all}
	for len(boxes) < n {
		cut, cutAxis, cutWidth := -1, 0, 0.0
		for i, box := range boxes {
			if axis, w := widest(box); len(box) > 1 && w > cutWidth {
				cut, cutAxis, cutWidth = i, axis, w
			}
		}
		if cut < 0 {
			break // every box is one colour
		}
		box := boxes[cut]
		slices.SortStableFunc(box, func(a, b px) int { return cmpFloat(a.lab[cutAxis], b.lab[cutAxis]) })
		m := len(box) / 2
		boxes = slices.Replace(boxes, cut, cut+1, box[:m], box[m:])
	}
	pal := make(color.Palette, len(boxes))
	for i, box := range boxes {
		var r, g, bl float64
		for _, p := range box {
			r, g, bl = r+float64(p.c.R), g+float64(p.c.G), bl+float64(p.c.B)
		}
		n := float64(len(box))
		pal[i] = color.NRGBA{uint8(r/n + 0.5), uint8(g/n + 0.5), uint8(bl/n + 0.5), 255}
	}
	return pal
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// bayer is the 4×4 ordered dithering matrix.
var bayer = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

// ditherSpread is how far dithering moves a channel, about half the gap
// between neighbouring palette colours.
const ditherSpread = 24

// Quantize maps img to pal by the nearest colour (CIELAB), with ordered
// dithering if asked.
func Quantize(img *image.NRGBA, pal color.Palette, dither bool) *image.Paletted {
	b := img.Bounds()
	out := image.NewPaletted(b, pal)
	labs := paletteLab(pal)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			r, g, bl := float64(c.R), float64(c.G), float64(c.B)
			if dither {
				d := (bayer[y%4][x%4]/16 - 0.5) * ditherSpread
				r, g, bl = r+d, g+d, bl+d
			}
			out.SetColorIndex(x, y, nearest(labs, r, g, bl))
		}
	}
	return out
}

// nearest is the palette index closest to r, g, b in CIELAB, where
// distance follows how far apart colours look. A plain RGB distance sent
// shaded skin to the palette's greys and blues.
func nearest(labs [][3]float64, r, g, b float64) uint8 {
	l, a, bb := lab(r, g, b)
	best, bestD := 0, math.Inf(1)
	for i, p := range labs {
		d := (l-p[0])*(l-p[0]) + (a-p[1])*(a-p[1]) + (bb-p[2])*(bb-p[2])
		if d < bestD {
			best, bestD = i, d
		}
	}
	return uint8(best)
}

// paletteLab is pal in CIELAB.
func paletteLab(pal color.Palette) [][3]float64 {
	out := make([][3]float64, len(pal))
	for i, pc := range pal {
		c := color.NRGBAModel.Convert(pc).(color.NRGBA)
		out[i][0], out[i][1], out[i][2] = lab(float64(c.R), float64(c.G), float64(c.B))
	}
	return out
}

// lab converts sRGB (0–255, clamped) to CIELAB under D65.
func lab(r, g, b float64) (l, a, bb float64) {
	lin := func(v float64) float64 {
		v = math.Min(math.Max(v, 0), 255) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	R, G, B := lin(r), lin(g), lin(b)
	x := (0.4124*R + 0.3576*G + 0.1805*B) / 0.95047
	y := 0.2126*R + 0.7152*G + 0.0722*B
	z := (0.0193*R + 0.1192*G + 0.9505*B) / 1.08883
	f := func(t float64) float64 {
		if t > 216.0/24389 {
			return math.Cbrt(t)
		}
		return (24389.0/27*t + 16) / 116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}

// readImage decodes an image file.
func readImage(path string) (image.Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// BuiltPortrait is a source with its pixel portraits.
type BuiltPortrait struct {
	PortraitSource
	Portrait
}

// BuildPortraits makes the pixel portraits of the fetched sources and
// writes them to work/out (<id>-32.png, <id>-20.png) with sheet.png, every
// portrait enlarged beside its crop, for review (SPEC §8.9).
func BuildPortraits(work string, sources []PortraitSource, ov PortraitOverrides, cfg ImageConfig, log io.Writer) ([]BuiltPortrait, error) {
	if err := os.MkdirAll(filepath.Join(work, "out"), 0o755); err != nil {
		return nil, err
	}
	var built []BuiltPortrait
	var noFace []string
	bar := newProgress(log, "portraits", len(sources))
	for i, s := range sources {
		bar.step(i+1, s.Name)
		img, err := readImage(filepath.Join(work, s.Image))
		if err != nil {
			return built, fmt.Errorf("%s (%d): %w", s.Name, s.RikishiID, err)
		}
		p := MakePortrait(img, ov[s.RikishiID], cfg)
		if !p.Face {
			noFace = append(noFace, fmt.Sprintf("%s (%d)", s.Name, s.RikishiID))
		}
		for size, im := range p.Images {
			if err := writePNG(filepath.Join(work, "out", fmt.Sprintf("%d-%d.png", s.RikishiID, size)), im); err != nil {
				return built, err
			}
		}
		built = append(built, BuiltPortrait{s, p})
	}
	bar.done()
	if err := writePNG(filepath.Join(work, "sheet.png"), sheet(built, work, ov)); err != nil {
		return built, err
	}
	_, _ = fmt.Fprintf(log, "%d portraits built; no face found, cropped from the top: %d\n", len(built), len(noFace))
	for _, n := range noFace {
		_, _ = fmt.Fprintln(log, "  "+n)
	}
	return built, nil
}

// sheetCell is one portrait on the review sheet: the thumbnail's crop and
// the largest portrait, both 144 px.
const sheetCell = 144

// sheetLabel is the strip under each cell: the wrestler's ID and name, as
// overrides.toml needs them.
const sheetLabel = 16

// sheet lays the portraits out for review, 8 a row: the crop of the photo,
// then the largest pixel portrait enlarged, over a label with the ID and
// name; "no face" marks a crop from the top, "override" a manual one.
func sheet(built []BuiltPortrait, work string, ov PortraitOverrides) image.Image {
	const perRow = 8
	rowH := sheetCell + sheetLabel
	rows := max((len(built)+perRow-1)/perRow, 1)
	out := image.NewNRGBA(image.Rect(0, 0, perRow*2*sheetCell, rows*rowH))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.NRGBA{24, 24, 28, 255}), image.Point{}, draw.Src)
	for i, b := range built {
		x, y := (i%perRow)*2*sheetCell, (i/perRow)*rowH
		if img, err := readImage(filepath.Join(work, b.Image)); err == nil {
			crop := AreaScale(img, b.Crop, sheetCell)
			draw.Draw(out, image.Rect(x, y, x+sheetCell, y+sheetCell), crop, image.Point{}, draw.Src)
		}
		big := portrait.Sizes[0]
		scale := sheetCell / big
		for py := range big {
			for px := range big {
				c := b.Images[big].At(px, py)
				draw.Draw(out, image.Rect(x+sheetCell+px*scale, y+py*scale, x+sheetCell+(px+1)*scale, y+(py+1)*scale), image.NewUniform(c), image.Point{}, draw.Src)
			}
		}
		label := fmt.Sprintf("%d %s", b.RikishiID, b.Name)
		switch _, manual := ov[b.RikishiID]; {
		case manual:
			label += " (override)"
		case !b.Face:
			label += " (no face)"
		}
		d := font.Drawer{
			Dst: out, Src: image.NewUniform(color.White), Face: basicfont.Face7x13,
			Dot: fixed.P(x+4, y+sheetCell+12),
		}
		d.DrawString(label)
	}
	return out
}

func writePNG(path string, img image.Image) error {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return atomicfile.Write(path, b.Bytes())
}

// WritePortraitData replaces the portraits in dir (data/portraits) with
// built: <size>/<id>.png, credits.json, CREDITS.md and LICENSE. Portraits of
// an earlier run that are not in built go; overrides.toml stays.
func WritePortraitData(dir string, built []BuiltPortrait) error {
	for _, size := range portrait.Sizes {
		sub := filepath.Join(dir, strconv.Itoa(size))
		if err := os.RemoveAll(sub); err != nil {
			return err
		}
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return err
		}
	}
	credits := make([]portrait.Credit, 0, len(built))
	for _, b := range built {
		for size, im := range b.Images {
			if err := writePNG(filepath.Join(dir, portrait.Path(b.RikishiID, size)), im); err != nil {
				return err
			}
		}
		credits = append(credits, portrait.Credit{
			RikishiID: b.RikishiID, Name: b.Name, File: b.File, Author: b.Author,
			Licence: b.Licence, LicenceURL: b.LicenceURL, SourceURL: b.SourceURL, Year: b.Year,
		})
	}
	slices.SortFunc(credits, func(a, b portrait.Credit) int { return int(a.RikishiID - b.RikishiID) })
	j, err := json.MarshalIndent(credits, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicfile.Write(filepath.Join(dir, "credits.json"), append(j, '\n')); err != nil {
		return err
	}
	if err := atomicfile.Write(filepath.Join(dir, "CREDITS.md"), []byte(creditsMarkdown(credits))); err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(dir, "LICENSE"), []byte(portraitLicence))
}

// creditsMarkdown is CREDITS.md: every photo with its author, licence and
// source, as the licences ask.
func creditsMarkdown(credits []portrait.Credit) string {
	var b strings.Builder
	b.WriteString("# Portrait credits\n\nGenerated by `basho-data portraits update`; do not edit. ")
	fmt.Fprintf(&b, "Each portrait is a photo from Wikimedia Commons, %s, and keeps the photo's licence (see the Licence column).\n\n", portrait.Changes)
	b.WriteString("| Wrestler | Photo | Author | Licence |\n|---|---|---|---|\n")
	esc := strings.NewReplacer("|", `\|`, "\n", " ")
	for _, c := range credits {
		licence := esc.Replace(c.Licence)
		if c.LicenceURL != "" {
			licence = fmt.Sprintf("[%s](%s)", licence, c.LicenceURL)
		}
		fmt.Fprintf(&b, "| %s | [%s](%s) | %s | %s |\n", esc.Replace(c.Name), esc.Replace(c.File), c.SourceURL, esc.Replace(c.Author), licence)
	}
	return b.String()
}

const portraitLicence = `The images in this directory are adaptations of photos from Wikimedia
Commons: ` + portrait.Changes + `. Each keeps the licence of the photo it
comes from, mostly Creative Commons Attribution-ShareAlike (CC BY-SA); a few
are CC BY, CC0 or in the public domain. CREDITS.md and credits.json list every
photo with its author, licence and source; ` + "`basho credits --portraits`" + ` prints
the same list.

This licence covers the images only. basho's code has its own licence.
`
