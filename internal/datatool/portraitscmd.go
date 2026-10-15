package datatool

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

func newPortraitsCmd(info buildinfo.Info) *cobra.Command {
	var cfg PortraitConfig
	var only, snapDir, workCache, contact, overrides, out string
	var img ImageConfig
	var overridesOnly bool
	update := &cobra.Command{
		Use:   "update",
		Short: "Fetch wrestler portraits from Wikimedia Commons (about one request per image)",
		Long: "update finds the snapshot's wrestlers on Wikidata (JSA ID, P3385) with an image\n" +
			"(P18), keeps the images whose licence is on the allow-list, and fetches 500 px\n" +
			"thumbnails into --work, one request per second per host (SPEC §4.4, §8.9).\n" +
			"Then it crops each to head and shoulders around the face, reduces it to 48, 32\n" +
			"and 20 px in a palette of its own --colours, and writes them with a review\n" +
			"sheet (sheet.png).\n" +
			"Responses go through a work cache, so a second run fetches nothing again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if img.Colours < 2 || img.Colours > 256 {
				return fmt.Errorf("--colours: %d, want 2 to 256", img.Colours)
			}
			for f := range strings.SplitSeq(only, ",") {
				if f = strings.TrimSpace(f); f != "" {
					id, err := strconv.Atoi(f)
					if err != nil {
						return fmt.Errorf("--only: %q is not a wrestler ID", f)
					}
					cfg.Only = append(cfg.Only, domain.RikishiID(id))
				}
			}
			s, err := snapshot.Open(os.DirFS(snapDir))
			if err != nil {
				return err
			}
			rs, err := s.Rikishi()
			if err != nil {
				return err
			}
			c, err := cache.Open(workCache, 4<<30)
			if err != nil {
				return err
			}
			if cfg.Overrides, err = LoadPortraitOverrides(overrides); err != nil {
				return err
			}
			if overridesOnly {
				if len(cfg.Overrides) == 0 {
					return fmt.Errorf("--overrides-only: %s has no overrides", overrides)
				}
				cfg.Only = slices.Sorted(maps.Keys(cfg.Overrides))
			}
			g := &cache.Getter{Cache: c, Next: newWikimediaClient(info, contact), Policy: wikimediaPolicy}
			set, err := FetchPortraits(cmd.Context(), g, rs, cfg, cmd.ErrOrStderr())
			if err != nil || cfg.DryRun {
				return err
			}
			built, err := BuildPortraits(cfg.Work, set.Sources, cfg.Overrides, img, cmd.ErrOrStderr())
			if err != nil || len(cfg.Only) > 0 || cfg.Limit > 0 {
				return err // a partial run never replaces the shipped set
			}
			if err := WritePortraitData(out, built); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.ErrOrStderr(), "%d portraits written to %s\n", len(built), out)
			return err
		},
	}
	f := update.Flags()
	f.StringVar(&cfg.Work, "work", ".cache/basho-data/portraits", "directory for the thumbnails and their sources")
	f.StringVar(&snapDir, "snapshot", "data/snapshot", "snapshot directory: whose portraits to look for")
	f.StringVar(&workCache, "cache", ".cache/basho-data", "work cache directory")
	f.StringVar(&only, "only", "", "only these wrestler IDs, comma separated")
	f.IntVar(&cfg.Limit, "limit", 0, "at most this many images, for a smoke run")
	f.BoolVar(&cfg.DryRun, "dry-run", false, "run the Wikidata query (one request) and print the plan")
	f.StringVar(&overrides, "overrides", "data/portraits/overrides.toml", "manual crops, exclusions and other files")
	f.StringVar(&out, "out", "data/portraits", "where the portraits and credits go (not with --only or --limit)")
	f.BoolVar(&overridesOnly, "overrides-only", false, "only the wrestlers in --overrides, for a quick look at them (use another --work)")
	f.IntVar(&img.Colours, "colours", DefaultColours, "colours per portrait")
	f.BoolVar(&img.Dither, "dither", false, "ordered dithering when reducing the colours")
	f.StringVar(&contact, "contact", "https://github.com/thorstenpfister/basho",
		"contact for the User-Agent, as Wikimedia's policy asks: a URL or an email address")
	portraits := &cobra.Command{Use: "portraits", Short: "Wrestler portraits from Wikimedia Commons (SPEC §8.9)"}
	portraits.AddCommand(update)
	return portraits
}

// newWikimediaClient is polite to Wikimedia (SPEC §4.4): one request per
// second per host, and a User-Agent with a contact.
func newWikimediaClient(info buildinfo.Info, contact string) *httpx.Client {
	hosts := map[string]httpx.HostPolicy{}
	// Commons hands out thumbnails on thumb.wikimedia.org, formerly upload.
	for _, h := range []string{host(WikidataSPARQL), host(CommonsAPI), "thumb.wikimedia.org", "upload.wikimedia.org"} {
		hosts[h] = httpx.HostPolicy{Interval: time.Second, MaxConcurrent: 1}
	}
	return httpx.New(httpx.Options{UserAgent: fmt.Sprintf("basho-data/%s (%s)", info.Version, contact), Hosts: hosts})
}

// wikimediaPolicy keeps the query and the metadata for a week, so a later
// run picks up new photos, and the thumbnails for good.
func wikimediaPolicy(url string, status int, _ []byte, _ time.Time) (time.Duration, bool) {
	switch {
	case status != 200:
		return 0, false
	case strings.HasPrefix(url, WikidataSPARQL), strings.HasPrefix(url, CommonsAPI):
		return 7 * 24 * time.Hour, false
	default:
		return 0, true
	}
}
