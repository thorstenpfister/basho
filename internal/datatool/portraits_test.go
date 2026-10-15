package datatool

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
)

// wikimedia answers like Wikidata, Commons and their thumbnail host, from
// the shapes the real services gave on 2026-10-15.
type wikimedia struct {
	sparql, commons string
	thumbs          map[string][]byte
	asked           []string
}

func (w *wikimedia) Get(_ context.Context, u string, _ httpx.Validators) (*httpx.Response, error) {
	w.asked = append(w.asked, u)
	switch {
	case strings.HasPrefix(u, WikidataSPARQL):
		return &httpx.Response{Status: 200, Body: []byte(w.sparql)}, nil
	case strings.HasPrefix(u, CommonsAPI):
		return &httpx.Response{Status: 200, Body: []byte(w.commons)}, nil
	}
	if b, ok := w.thumbs[u]; ok {
		return &httpx.Response{Status: 200, Body: b}, nil
	}
	return &httpx.Response{Status: 404}, nil
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 6))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const sparqlAnswer = `{"results":{"bindings":[
 {"item":{"value":"http://www.wikidata.org/entity/Q1"},"sdb":{"value":"100"},"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/Onosato%202024.jpg"}},
 {"item":{"value":"http://www.wikidata.org/entity/Q1"},"sdb":{"value":"100"},"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/Zzz%20later.jpg"}},
 {"item":{"value":"http://www.wikidata.org/entity/Q2"},"jsa":{"value":"3842"},"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/Hoshoryu.png"}},
 {"item":{"value":"http://www.wikidata.org/entity/Q3"},"sdb":{"value":"300"},"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/NC%20only.jpg"}},
 {"item":{"value":"http://www.wikidata.org/entity/Q4"},"sdb":{"value":"400"},"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/Old%20scan.jpg"}},
 {"item":{"value":"http://www.wikidata.org/entity/Q5"},"sdb":{"value":"500"},"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/Broken.jpg"}},
 {"item":{"value":"http://www.wikidata.org/entity/Q9"},"jsa":{"value":"no number"},"image":{"value":"http://commons.wikimedia.org/wiki/Special:FilePath/X.jpg"}}
]}}`

const commonsAnswer = `{"batchcomplete":true,"query":{
 "normalized":[{"from":"File:Onosato_2024.jpg","to":"File:Onosato 2024.jpg"}],
 "pages":[
  {"title":"File:Onosato 2024.jpg","imageinfo":[{"thumburl":"https://thumb.wikimedia.org/o.png","descriptionurl":"https://commons.wikimedia.org/wiki/File:Onosato_2024.jpg",
   "extmetadata":{"License":{"value":"cc-by-sa-4.0"},"LicenseShortName":{"value":"CC BY-SA 4.0"},"LicenseUrl":{"value":"https://creativecommons.org/licenses/by-sa/4.0"},
    "Artist":{"value":"<a href=\"//commons.wikimedia.org/wiki/User:X\">Photo &amp; Co</a>"},"DateTimeOriginal":{"value":"<span>2024-05-12</span> 15:00"}}}]},
  {"title":"File:Hoshoryu.png","imageinfo":[{"thumburl":"https://thumb.wikimedia.org/h.png","descriptionurl":"https://commons.wikimedia.org/wiki/File:Hoshoryu.png",
   "extmetadata":{"LicenseShortName":{"value":"Public domain"},"Artist":{"value":"Unknown"}}}]},
  {"title":"File:NC only.jpg","imageinfo":[{"thumburl":"https://thumb.wikimedia.org/nc.png","descriptionurl":"x",
   "extmetadata":{"License":{"value":"cc-by-nc-2.0"},"LicenseShortName":{"value":"CC BY-NC 2.0"}}}]},
  {"title":"File:Old scan.jpg","imageinfo":[{"thumburl":"https://thumb.wikimedia.org/old.png","descriptionurl":"y",
   "extmetadata":{"License":{"value":"cc-by-sa-2.1-jp"},"LicenseShortName":{"value":"CC BY-SA 2.1 jp"},"DateTimeOriginal":{"value":1959}}}]},
  {"title":"File:Broken.jpg","imageinfo":[{"thumburl":"https://thumb.wikimedia.org/broken.png","descriptionurl":"z",
   "extmetadata":{"License":{"value":"cc0"}}}]}
 ]}}`

func TestFetchPortraits(t *testing.T) {
	img := pngBytes(t)
	w := &wikimedia{sparql: sparqlAnswer, commons: commonsAnswer, thumbs: map[string][]byte{
		"https://thumb.wikimedia.org/o.png": img, "https://thumb.wikimedia.org/h.png": img,
		"https://thumb.wikimedia.org/nc.png": img, "https://thumb.wikimedia.org/old.png": img,
		"https://thumb.wikimedia.org/broken.png": []byte("not an image"),
	}}
	rs := []domain.Rikishi{
		{ID: 5, SumoDBID: 500, Name: domain.Name{En: "Broken"}},
		{ID: 1, SumoDBID: 100, NSKID: 1, Name: domain.Name{En: "Onosato"}},     // by SumoDB ID
		{ID: 2, SumoDBID: 999, NSKID: 3842, Name: domain.Name{En: "Hoshoryu"}}, // by JSA ID
		{ID: 3, SumoDBID: 300, Name: domain.Name{En: "Noncommercial"}},
		{ID: 4, SumoDBID: 400, Name: domain.Name{En: "Oldtimer"}},
		{ID: 6, SumoDBID: 600, Name: domain.Name{En: "Nophoto"}},
	}
	work := t.TempDir()
	set, err := FetchPortraits(t.Context(), w, rs, PortraitConfig{Work: work}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range set.Sources {
		got = append(got, s.Name+" "+s.File+" "+s.Licence+" "+s.Author)
	}
	want := []string{
		"Onosato Onosato 2024.jpg CC BY-SA 4.0 Photo & Co", // the first file name of two, the normalised title, plain text
		"Hoshoryu Hoshoryu.png Public domain Unknown",
		"Oldtimer Old scan.jpg CC BY-SA 2.1 jp ", // a ported version is allowed
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("sources:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if s := set.Sources[0]; s.Year != 2024 || s.Item != "Q1" || s.Image != filepath.Join("images", "1.png") || s.SourceURL == "" {
		t.Errorf("Onosato: %+v", s)
	}
	if set.Sources[2].Year != 1959 {
		t.Errorf("a numeric date: %+v", set.Sources[2])
	}
	var reasons []string
	for _, s := range set.Skipped {
		reasons = append(reasons, s.Name+": "+s.Reason)
	}
	if r := strings.Join(reasons, "\n"); !strings.Contains(r, `Noncommercial: licence "CC BY-NC 2.0" is not on the allow-list`) || !strings.Contains(r, "Broken: thumbnail:") || len(reasons) != 2 {
		t.Errorf("skipped:\n%s", r)
	}
	if _, err := os.Stat(filepath.Join(work, "images", "1.png")); err != nil {
		t.Error(err)
	}
	var saved []PortraitSource
	b, _ := os.ReadFile(filepath.Join(work, "sources.json"))
	if err := json.Unmarshal(b, &saved); err != nil || len(saved) != 3 {
		t.Errorf("sources.json: %d, %v", len(saved), err)
	}
	// One query, one metadata batch, four thumbnails: a licence off the list
	// is never downloaded. The titles go in one request.
	if len(w.asked) != 6 {
		t.Errorf("%d requests: %v", len(w.asked), w.asked)
	}
	if u, _ := url.QueryUnescape(w.asked[1]); !strings.Contains(u, "titles=File:Onosato 2024.jpg|File:Hoshoryu.png|") || !strings.Contains(u, "iiurlwidth=500") {
		t.Errorf("commons request %s", u)
	}
}

func TestFetchPortraitsSelection(t *testing.T) {
	w := &wikimedia{sparql: sparqlAnswer, commons: commonsAnswer}
	rs := []domain.Rikishi{{ID: 1, SumoDBID: 100}, {ID: 2, NSKID: 3842}, {ID: 4, SumoDBID: 400}}
	set, err := FetchPortraits(t.Context(), w, rs, PortraitConfig{Work: t.TempDir(), DryRun: true}, io.Discard)
	if err != nil || len(set.Sources) != 3 || len(w.asked) != 1 {
		t.Errorf("dry run: %d sources, %d requests, %v", len(set.Sources), len(w.asked), err)
	}
	for _, cfg := range []PortraitConfig{{Only: []domain.RikishiID{4}}, {Limit: 1}} {
		cfg.DryRun = true
		set, _ := FetchPortraits(t.Context(), w, rs, cfg, io.Discard)
		if len(set.Sources) != 1 {
			t.Errorf("%+v: %d sources", cfg, len(set.Sources))
		}
	}
	if PortraitRequests(360) != 1+8+360 {
		t.Errorf("requests for 360: %d", PortraitRequests(360))
	}
}

func TestLicences(t *testing.T) {
	for code, ok := range map[string]bool{
		"cc0": true, "pd": true, "cc-by-2.0": true, "cc-by-sa-4.0": true, "cc-by-sa-3.0-de": true, "cc-by-2.1-jp": true,
		"cc-by-nc-2.0": false, "cc-by-nd-4.0": false, "cc-by-sa-1.0": false, "gfdl": false, "": false, "fair use": false,
	} {
		if allowedLicence(code) != ok {
			t.Errorf("%q: %v", code, !ok)
		}
	}
	if licenceCode("", "Public domain") != "pd" || licenceCode("", "CC0") != "cc0" || licenceCode("CC-BY-4.0", "") != "cc-by-4.0" {
		t.Error("licenceCode")
	}
	if ttl, pinned := wikimediaPolicy(WikidataSPARQL+"?q", 200, nil, time.Time{}); pinned || ttl != 7*24*time.Hour {
		t.Error("the query is kept a week")
	}
	if _, pinned := wikimediaPolicy("https://thumb.wikimedia.org/x.jpg", 200, nil, time.Time{}); !pinned {
		t.Error("thumbnails are kept for good")
	}
}
