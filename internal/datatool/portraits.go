package datatool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/jpeg" // Commons thumbnails
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/thorstenpfister/basho/internal/atomicfile"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
)

// Portrait sources (SPEC §4.4, §8.9): Wikidata joins a wrestler to an image
// on Commons (P18) by the SumoDB ID (P6652, which sumo-api also gives) or
// the JSA ID (P3385); Commons gives the image's licence, author and a
// thumbnail. Only basho-data talks to Wikimedia, never the app.
const (
	WikidataSPARQL = "https://query.wikidata.org/sparql"
	CommonsAPI     = "https://commons.wikimedia.org/w/api.php"
	// ThumbWidth is one of Wikimedia's standard thumbnail widths, which its
	// caches serve; plenty for 32 px portraits.
	ThumbWidth   = 500
	commonsBatch = 50 // titles per Commons API request, its limit
)

// portraitQuery lists every Wikidata item with a SumoDB or JSA ID and an
// image. Most older wrestlers have no JSA ID in sumo-api's index, but
// nearly all have a SumoDB ID.
const portraitQuery = `SELECT ?item ?sdb ?jsa ?image WHERE { { ?item wdt:P6652 ?sdb } UNION { ?item wdt:P3385 ?jsa } ?item wdt:P18 ?image . }`

// PortraitSource is a wrestler's image on Commons with what crediting it
// needs (SPEC §8.9).
type PortraitSource struct {
	RikishiID  domain.RikishiID `json:"id"`
	Name       string           `json:"name"`
	Item       string           `json:"item"`    // Wikidata item, e.g. Q123
	File       string           `json:"file"`    // Commons file name, without "File:"
	Licence    string           `json:"licence"` // e.g. "CC BY-SA 4.0"
	LicenceURL string           `json:"licenceUrl,omitempty"`
	Author     string           `json:"author"`         // plain text
	Date       string           `json:"date,omitempty"` // when the photo was taken, as Commons says
	Year       int              `json:"year,omitempty"` // from Date, for the caption
	SourceURL  string           `json:"sourceUrl"`      // the file's page on Commons
	Thumb      string           `json:"thumb"`          // the thumbnail fetched
	Image      string           `json:"image"`          // its file in the work directory
}

// PortraitSkip is a wrestler with an image that was not taken, and why.
type PortraitSkip struct {
	RikishiID domain.RikishiID `json:"id"`
	Name      string           `json:"name"`
	File      string           `json:"file"`
	Reason    string           `json:"reason"`
}

// PortraitConfig selects the wrestlers and where the fetch goes.
type PortraitConfig struct {
	Work   string             // directory for the thumbnails, sources.json and skipped.json
	Only   []domain.RikishiID // just these wrestlers; empty: all
	Limit  int                // at most this many images, for a smoke run; zero: all
	DryRun bool               // the Wikidata query only, then the plan
	// Overrides exclude wrestlers or pick another file (overrides.toml).
	Overrides PortraitOverrides
}

// PortraitSet is what a fetch found.
type PortraitSet struct {
	Sources []PortraitSource
	Skipped []PortraitSkip
}

// PortraitRequests is the number of requests a fetch of n images makes:
// the query, the Commons metadata in batches, and one thumbnail each.
func PortraitRequests(n int) int { return 1 + (n+commonsBatch-1)/commonsBatch + n }

// FetchPortraits finds the images of the wrestlers in rs on Wikidata, keeps
// those with an allowed licence, and fetches their thumbnails into
// cfg.Work. g should be polite (1 request per second per host) and cached.
func FetchPortraits(ctx context.Context, g httpx.Getter, rs []domain.Rikishi, cfg PortraitConfig, log io.Writer) (PortraitSet, error) {
	images, err := wikidataImages(ctx, g)
	if err != nil {
		return PortraitSet{}, err
	}
	var want []PortraitSource
	var set PortraitSet
	for _, r := range rs {
		img, ok := images.of(r)
		o := cfg.Overrides[r.ID]
		if o.File != "" {
			img.file, ok = o.File, true
		}
		if !ok || len(cfg.Only) > 0 && !slices.Contains(cfg.Only, r.ID) {
			continue
		}
		if o.Exclude {
			set.Skipped = append(set.Skipped, PortraitSkip{RikishiID: r.ID, Name: r.Name.En, File: img.file, Reason: "excluded in overrides.toml: " + o.Note})
			continue
		}
		want = append(want, PortraitSource{RikishiID: r.ID, Name: r.Name.En, Item: img.item, File: img.file})
	}
	slices.SortFunc(want, func(a, b PortraitSource) int { return int(a.RikishiID - b.RikishiID) })
	if cfg.Limit > 0 && len(want) > cfg.Limit {
		want = want[:cfg.Limit]
	}
	_, _ = fmt.Fprintf(log, "%d wrestlers with an image on Wikidata; up to %d more requests, about %s (the work cache answers what an earlier run fetched)\n",
		len(want), PortraitRequests(len(want))-1, time.Duration(PortraitRequests(len(want))-1)*time.Second)
	if cfg.DryRun {
		return PortraitSet{Sources: want}, nil
	}

	files := make([]string, len(want))
	for i, w := range want {
		files[i] = w.File
	}
	infos, err := commonsInfos(ctx, g, files, log)
	if err != nil {
		return PortraitSet{}, err
	}
	if err := os.MkdirAll(filepath.Join(cfg.Work, "images"), 0o755); err != nil {
		return PortraitSet{}, err
	}
	bar := newProgress(log, "thumbnails", len(want))
	defer bar.done()
	for i, w := range want {
		bar.step(i+1, w.Name)
		skip := func(reason string) {
			set.Skipped = append(set.Skipped, PortraitSkip{RikishiID: w.RikishiID, Name: w.Name, File: w.File, Reason: reason})
			bar.done()
			_, _ = fmt.Fprintf(log, "skip %d %s: %s\n", w.RikishiID, w.Name, reason)
		}
		info, ok := infos[w.File]
		switch {
		case !ok || info.Thumb == "":
			skip("not on Commons")
			continue
		case !allowedLicence(info.LicenceCode):
			skip(fmt.Sprintf("licence %q is not on the allow-list", orElse(info.Licence, info.LicenceCode)))
			continue
		}
		resp, err := g.Get(ctx, info.Thumb, httpx.Validators{})
		if err != nil {
			return set, err
		}
		if !resp.OK() {
			skip(fmt.Sprintf("thumbnail: HTTP %d", resp.Status))
			continue
		}
		_, format, err := image.DecodeConfig(bytes.NewReader(resp.Body))
		if err != nil {
			skip("thumbnail: " + err.Error())
			continue
		}
		w.Licence, w.LicenceURL, w.Author, w.Date, w.SourceURL, w.Thumb = info.Licence, info.LicenceURL, info.Author, info.Date, info.Page, info.Thumb
		w.Year = yearOf(info.Date)
		w.Image = filepath.Join("images", fmt.Sprintf("%d.%s", w.RikishiID, format))
		if err := atomicfile.Write(filepath.Join(cfg.Work, w.Image), resp.Body); err != nil {
			return set, err
		}
		set.Sources = append(set.Sources, w)
	}
	for name, v := range map[string]any{"sources.json": set.Sources, "skipped.json": set.Skipped} {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return set, err
		}
		if err := atomicfile.Write(filepath.Join(cfg.Work, name), append(b, '\n')); err != nil {
			return set, err
		}
	}
	bar.done()
	_, _ = fmt.Fprintf(log, "%d portraits fetched, %d skipped (%s)\n", len(set.Sources), len(set.Skipped), cfg.Work)
	return set, nil
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type wdImage struct{ item, file string }

// wdImages are the images by SumoDB ID and by JSA ID.
type wdImages struct{ bySumoDB, byJSA map[int]wdImage }

// of finds r's image: by SumoDB ID, then by JSA ID.
func (w wdImages) of(r domain.Rikishi) (wdImage, bool) {
	if img, ok := w.bySumoDB[r.SumoDBID]; ok && r.SumoDBID > 0 {
		return img, true
	}
	img, ok := w.byJSA[r.NSKID]
	return img, ok && r.NSKID > 0
}

// wikidataImages returns the items and images by ID. An item with several
// images takes the first file name, so runs agree.
func wikidataImages(ctx context.Context, g httpx.Getter) (wdImages, error) {
	u := WikidataSPARQL + "?format=json&query=" + url.QueryEscape(portraitQuery)
	resp, err := g.Get(ctx, u, httpx.Validators{})
	if err != nil {
		return wdImages{}, err
	}
	if !resp.OK() {
		return wdImages{}, fmt.Errorf("wikidata: HTTP %d", resp.Status)
	}
	var r struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &r); err != nil {
		return wdImages{}, fmt.Errorf("wikidata: %w", err)
	}
	out := wdImages{bySumoDB: map[int]wdImage{}, byJSA: map[int]wdImage{}}
	for _, b := range r.Results.Bindings {
		file, err := url.PathUnescape(path.Base(b["image"].Value)) // …/Special:FilePath/Name%20X.jpg
		if err != nil || file == "" {
			continue
		}
		img := wdImage{item: path.Base(b["item"].Value), file: file}
		for key, m := range map[string]map[int]wdImage{"sdb": out.bySumoDB, "jsa": out.byJSA} {
			id, err := strconv.Atoi(b[key].Value)
			if err != nil {
				continue // unbound, or not a number we can join
			}
			if old, ok := m[id]; !ok || img.file < old.file {
				m[id] = img
			}
		}
	}
	return out, nil
}

type commonsInfo struct {
	Thumb, Page              string
	LicenceCode, Licence     string
	LicenceURL, Author, Date string
}

// commonsInfos returns the Commons metadata of files, by file name, in
// batches.
func commonsInfos(ctx context.Context, g httpx.Getter, files []string, log io.Writer) (map[string]commonsInfo, error) {
	out := map[string]commonsInfo{}
	bar := newProgress(log, "metadata", (len(files)+commonsBatch-1)/commonsBatch)
	defer bar.done()
	n := 0
	for batch := range slices.Chunk(files, commonsBatch) {
		n++
		bar.step(n, "")
		titles := make([]string, len(batch))
		for i, f := range batch {
			titles[i] = "File:" + f
		}
		u := CommonsAPI + "?action=query&format=json&formatversion=2&prop=imageinfo&iiprop=url%7Cextmetadata" +
			"&iiurlwidth=" + strconv.Itoa(ThumbWidth) + "&titles=" + url.QueryEscape(strings.Join(titles, "|"))
		resp, err := g.Get(ctx, u, httpx.Validators{})
		if err != nil {
			return nil, err
		}
		if !resp.OK() {
			return nil, fmt.Errorf("commons: HTTP %d", resp.Status)
		}
		var r struct {
			Query struct {
				Normalized []struct{ From, To string } `json:"normalized"`
				Pages      []struct {
					Title     string `json:"title"`
					ImageInfo []struct {
						ThumbURL       string `json:"thumburl"`
						DescriptionURL string `json:"descriptionurl"`
						ExtMetadata    map[string]struct {
							Value any `json:"value"` // usually a string, sometimes a number
						} `json:"extmetadata"`
					} `json:"imageinfo"`
				} `json:"pages"`
			} `json:"query"`
		}
		if err := json.Unmarshal(resp.Body, &r); err != nil {
			return nil, fmt.Errorf("commons: %w", err)
		}
		asked := map[string]string{} // the API's title → our file name
		for _, f := range batch {
			asked["File:"+f] = f
		}
		for _, n := range r.Query.Normalized {
			if f, ok := asked[n.From]; ok {
				asked[n.To] = f
			}
		}
		for _, p := range r.Query.Pages {
			f, ok := asked[p.Title]
			if !ok || len(p.ImageInfo) == 0 {
				continue
			}
			ii := p.ImageInfo[0]
			meta := func(key string) string {
				v, ok := ii.ExtMetadata[key]
				if !ok {
					return ""
				}
				return plainText(fmt.Sprint(v.Value))
			}
			out[f] = commonsInfo{
				Thumb: ii.ThumbURL, Page: ii.DescriptionURL,
				LicenceCode: licenceCode(meta("License"), meta("LicenseShortName")), Licence: meta("LicenseShortName"),
				LicenceURL: meta("LicenseUrl"), Author: meta("Artist"), Date: meta("DateTimeOriginal"),
			}
		}
	}
	return out, nil
}

var (
	tags   = regexp.MustCompile(`<[^>]*>`)
	spaces = regexp.MustCompile(`\s+`)
	year   = regexp.MustCompile(`\b(1[89]\d\d|20\d\d)\b`)
	// allowed is the licence allow-list (SPEC §4.4): CC0, public domain, and
	// CC BY and BY-SA 2.0–4.0, ported versions included.
	allowed = regexp.MustCompile(`^(cc0|pd|cc-by(-sa)?-(2\.0|2\.1|2\.5|3\.0|4\.0)(-[a-z]{2,})?)$`)
)

// plainText turns Commons' HTML metadata into one line of text.
func plainText(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(tags.ReplaceAllString(s, " ")), " "))
}

// licenceCode is Commons' machine-readable licence ("cc-by-sa-4.0"), or one
// read from the short name ("CC BY-SA 4.0", "Public domain") when missing.
func licenceCode(code, short string) string {
	if code != "" {
		return strings.ToLower(code)
	}
	s := strings.ToLower(short)
	switch {
	case strings.HasPrefix(s, "public domain"):
		return "pd"
	case s == "cc0" || strings.HasPrefix(s, "cc0 "):
		return "cc0"
	}
	return strings.ReplaceAll(s, " ", "-")
}

func allowedLicence(code string) bool { return allowed.MatchString(code) }

// yearOf finds the year in a Commons date, or 0.
func yearOf(date string) int {
	y, _ := strconv.Atoi(year.FindString(date))
	return y
}
