package datatool

import (
	"fmt"
	"os"
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
	var only, snapDir, workCache, contact string
	update := &cobra.Command{
		Use:   "update",
		Short: "Fetch wrestler portraits from Wikimedia Commons (about one request per image)",
		Long: "update finds the snapshot's wrestlers on Wikidata (JSA ID, P3385) with an image\n" +
			"(P18), keeps the images whose licence is on the allow-list, and fetches 500 px\n" +
			"thumbnails into --work, one request per second per host (SPEC §4.4, §8.9).\n" +
			"Responses go through a work cache, so a second run fetches nothing again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			g := &cache.Getter{Cache: c, Next: newWikimediaClient(info, contact), Policy: wikimediaPolicy}
			_, err = FetchPortraits(cmd.Context(), g, rs, cfg, cmd.ErrOrStderr())
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
