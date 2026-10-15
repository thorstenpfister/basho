# basho — Specification

> Status: **v0.20** (2026-10-15). Implemented: Phases 0–15 (Phase 10 step 4 and its real-day check wait for Kyushu 2026; Phase 12's QA pass moved to Phase 16; Phase 13's rating divisor was dropped as not needed). The full snapshot (196001–202609), the calibrated rating and 353 wrestler portraits are committed. The non-interactive CLI works offline, `basho wrestler` included. The TUI has the picker, the day view with its recap, the VS screen with portraits, the standings, the wrestler screen and a paged help overlay, and offers `ui.names` and plain modes. The current tournament comes from osada.us and sumo-api through the resolver, live mode polls it, and spoiler-free mode and `basho watch` work. Next: the QA pass (16), then the v1.0 release (17).
> Repository: `github.com/thorstenpfister/basho` · Module: `github.com/thorstenpfister/basho` · Binary: `basho`

---

## 1. Purpose

A terminal app for browsing the **days and bouts of a grand sumo tournament (basho)**. You move from day to day, look at individual bouts, and get a fighting-game-style "tale of the tape" screen before each bout. It also covers:

- the standings after any day
- a polite **live mode** that refreshes results automatically every few minutes during tournament days
- a small set of **non-interactive subcommands** with optional `--json` output

### 1.1 Goals

- Browse any tournament from **January 1960 to today**, in the **Makuuchi** and **Juryo** divisions.
- **Day view:** every bout of a day, in order, with results.
- **Bout view ("VS screen"):** name, rank, stable, hometown, age, record going into the bout, last 5 results, height and weight, kinboshi stars, yusho count, head-to-head record, technique profile (fighter type, style, signature move). The result can be revealed on the screen.
- **Standings view:** state after day N. Includes the leader board (yusho race), the classic 15-day results grid (hoshitori), and winning/losing record (kachi-koshi/make-koshi) markers.
- **Live mode:** automatic updates during the live window on tournament days, with strict limits on how often it calls the APIs.
- **Optional spoiler-free mode** for people watching recordings later.
- **Multiple data sources** behind one domain model, with fallback between them.
- **Bundled history:** finished tournaments since 1960 ship inside the binary as a data snapshot (§4.3), so browsing history needs no network.
- **Rating and win chance:** an Elo-style rating per wrestler, computed from the bundled history, with a win probability before each bout (§6.8).
- **Being a good citizen:** aggressive caching, rate limits, conditional requests, an identifying User-Agent.

### 1.2 Out of scope

Excluded entirely (not planned):

- Lower divisions (Makushita and below).
- Windows.
- The JSA's internal endpoints and commercial data providers. Only free, public APIs are used.
- Betting odds or anything presented as betting advice. The win chance (§6.8) is an estimate for fun.

Not in v1 (possible later, §15):

- Favourites, watchlists, notifications.
- A search screen or wrestler encyclopedia. (The wrestler screen, Phase 15, opens from bouts and standings.)

### 1.3 Decisions so far

| # | Decision | Choice |
|---|---|---|
| D1 | Language and TUI stack | Go, Charm v2 suite (Bubble Tea, Lip Gloss, Bubbles, Harmonica, Fang) |
| D2 | VS screen layout | "Tale of the tape": mirrored fighters around a centre column of stat labels |
| D3 | Divisions | Makuuchi and Juryo, toggled in the UI |
| D4 | Spoilers | Results shown by default; spoiler-free mode can be configured |
| D5 | Names | Romaji shown prominently, kanji alongside; `ui.names` switches to romaji only (`en`) or kanji first (`jp`) |
| D6 | History | Every tournament since 1960-01, fetched only when needed and cached |
| D7 | Non-TUI interface | Plain-text subcommands and `--json` |
| D8 | Favourites | None in v1 |
| D9 | Distribution | v1.0: private repository, local install and GoReleaser builds. Homebrew tap and public repository after 1.0 (D22, D25). |
| D10 | Data sources | sumo-api.com is primary for history; osada.us is primary for the live current tournament; each falls back to the other |
| D11 | Name | Binary, repository and module are all `basho`. Because the binary is named `basho`, the tournament flag is `-t/--tournament` and the tournament subcommands are `list` and `info` (no `basho basho …`). |
| D12 | VS screen extras | FROM and AGE kept; technique profile added (STYLE, SIGNATURE, fighter type, `m` moveset panel, §6.7) |
| D13 | Live mode divisions | Juryo and Makuuchi; window starts 14:00 JST |
| D14 | Spoiler-free default | New tournaments start with nothing revealed (Day 0) |
| D15 | Labels | English only for v1; the label strings are kept in one place so a bilingual option can be added later |
| D16 | Head-to-head key | `v` ("versus") on the VS screen, so `h` keeps meaning left/previous on every screen |
| D17 | Technique categories | Five categories as drafted: PUSH, BELT, THROW (including twists and trips), PULL, OTHER |
| D18 | Tournament argument | Accepts `202609`, `2026-09`, `aki2026`, `aki-2026` and the relative forms `current`, `prev`, `next` |
| D19 | Out of scope | Lower divisions, Windows, the JSA's internal endpoints and commercial providers are excluded entirely |
| D20 | Bundled snapshot | A separate tool (`basho-data`) builds the finished tournaments in advance; the result is embedded in the binary. Only the tournaments since the snapshot and the live tournament are fetched (§4.3, §5.8). |
| D21 | Rating | Our own Elo-style rating computed from the snapshot, with a win chance on the VS screen (§6.8). There is no public sumo rating API. |
| D22 | Homebrew tap | **After 1.0.** Hosted in this repository (`Casks/`), not in a separate tap repository (§14.3) |
| D23 | Portraits | Optional pixel-art portraits from Wikimedia Commons, built when the snapshot is built; **before 1.0** since 2026-10-15 (§4.4, §8.9, Phase 14) |
| D24 | sumo-api.com usage | No contact with the maintainer. Our use stays within fair use: one-off snapshot build at ≤ 1 request per second, incremental updates, aggressive caching. The snapshot is committed to this repository. |
| D25 | Signing and notarization | Not for now; after 1.0 at the earliest (§15). The binary works unsigned, as with many CLI tools. |
| D26 | Win chance | Shown on the VS screen by default (`ui.win_chance = true`); the day-view column stays opt-in (`p`) |
| D27 | Build tooling | A `Makefile` (no `just`), with the targets in §14.1. Third-party dependencies are kept to the short list in §5.9. |
| D28 | No GitHub Actions | All checks, builds, snapshot updates and releases run locally through the Makefile (`make check` before committing). There are no workflow files. |

---

## 2. Glossary

| Term | Meaning |
|---|---|
| Basho | Tournament. Six a year (Jan, Mar, May, Jul, Sep, Nov), 15 days each. ID format `YYYYMM`. |
| Honbasho | An official tournament (as opposed to exhibition tours). |
| Banzuke | Ranking list for a tournament, split into East and West sides. |
| Torikumi | The schedule of bouts for one day. |
| Hoshitori | The results grid (○ win, ● loss). |
| Makuuchi | Top division, about 42 wrestlers. Ranks: Yokozuna, Ozeki, Sekiwake, Komusubi (together "san'yaku"), then Maegashira. |
| Juryo | Second division, 28 wrestlers. |
| Rikishi | Wrestler. |
| Sekitori | A wrestler in Makuuchi or Juryo. "Sekitori bouts" are bouts in these two divisions, the only ones this app stores. |
| Elo | A rating system: points move from the loser to the winner, more when the result was unexpected. The rating difference translates directly into a win probability. |
| Shikona | Ring name. |
| Heya | Stable a wrestler belongs to. |
| Kimarite | Winning technique, for example *yorikiri* or *oshidashi*. |
| Fusen | Win or loss by default because a wrestler didn't show up. Shown as □ (win) and ■ (loss). |
| Kyujo | Absence. Shown as `–`. |
| Kachi-koshi / Make-koshi | A winning record (8+ wins) or losing record (8+ losses) in a 15-day tournament. |
| Kinboshi | "Gold star": a Maegashira beats a Yokozuna in an official tournament. A fusen win doesn't count. |
| Yusho | Tournament championship. |
| Sansho | Special prizes: Shukun-sho, Kanto-sho, Gino-sho. |
| Kettei-sen | Playoff after day 15 when wrestlers are tied for the lead. |
| Musubi-no-ichiban | Last bout of the day. |
| Senshuraku | Day 15, the final day. |
| JST | Japan Standard Time, UTC+9, no daylight saving. All tournament logic runs in JST. |

Tournament names by month: Hatsu (Jan, Tokyo), Haru (Mar, Osaka), Natsu (May, Tokyo), Nagoya (Jul, Nagoya), Aki (Sep, Tokyo), Kyushu (Nov, Fukuoka).

---

## 3. User experience overview

```
basho                      → opens the TUI at the current or most recent tournament, today or the latest day
basho --tournament 202609 --day 13
basho day 13               → prints the bouts of day 13 in the terminal
basho standings --day 10 --json
```

Flow in the TUI:

```
Basho picker ──enter──▶ Day view ──enter──▶ VS screen
     ▲                    │  ▲                 │
     └──────esc───────────┘  └──────esc────────┘
                          │
                          s ──▶ Standings (after the selected day) ──esc──▶ back
```

---

## 4. Data sources

### 4.1 sumo-api.com (primary for history, profiles and statistics)

- Base URL: `https://www.sumo-api.com/api`. No authentication, CORS open, no published rate limits. Run by a volunteer, so we keep our usage low.
- Coverage that has been checked:
  - Tournaments from `196001` onwards. Earlier tournaments return empty objects.
  - All six divisions.
  - 9,132 wrestlers including retired ones, 601 active (2026-10).
- Future tournaments return a "zero object": `date: ""` and `startDate: 0001-01-01`. Treat this as **not found**.

Endpoints used:

| Endpoint | Used for | Notes |
|---|---|---|
| `GET /basho/{id}` | Tournament metadata: start/end dates, yusho winner per division, special prizes | `location` is only present on some older tournaments, so derive the venue from the month |
| `GET /basho/{id}/banzuke/{division}` | Ranking plus a **15-entry `record` per wrestler** (result, opponent ID and names, kimarite) and the totals wins/losses/absences | `result` takes the values `win`, `loss`, `fusen win`, `fusen loss`, `absent` (kimarite is `fusen` or empty for the last three). Older tournaments may also have draws; this needs checking (§16 V3). |
| `GET /basho/{id}/torikumi/{division}/{day}` | The bout list for a day in order: `matchNo`, both wrestlers' IDs, shikona and ranks, `kimarite`, `winnerId` | Makuuchi has about 19–21 bouts a day, Juryo about 14. Day 15 may contain playoff bouts (July 2026 had 20 entries on day 15; needs checking, §16 V2). |
| `GET /rikishis?intai=true&limit=1000&skip=n` | Bulk wrestler index: `id`, `nskId`, `sumodbId`, shikona, heya, birth date, hometown, height, weight, debut | About 10 pages for everyone. Used to map osada.us IDs to sumo-api.com IDs and to fill profile fields without one request per wrestler. |
| `GET /rikishi/{id}` | A single profile, with optional `?ranks=true&shikonas=true&measurements=true` | Only used when the bulk index is missing or stale for that wrestler |
| `GET /rikishi/{id}/matches` | Full career of matches, newest first. **Each match includes `eastRank` and `westRank`.** | Basis for kinboshi, last 5 results across tournaments, and head-to-head records as of a given date. One request per wrestler, cached and topped up incrementally. |
| `GET /rikishi/{id}/stats` | Career totals: yusho, sansho, wins, losses, absences by division | These are **today's** values; see §6.6 for how this affects older tournaments |
| ~~`GET /measurements?bashoId={id}`~~ | Not used: it only lists the tournament's **newcomers**, because sumo-api records height and weight once, at a wrestler's debut (Phase 1 finding) | Height and weight come from the wrestler index (current values) |
| `GET /rikishi/{a}/matches/{b}` | Head-to-head summary and list of matches | Optional; can be computed from the cached matches instead |

Freshness: tournament data is updated during the tournament (some profiles had `updatedAt` 2026-09-17). How quickly bout results appear is **not documented and not yet measured** (§16 V1).

### 4.2 osada.us / o-sumo (primary for the live current tournament)

- Base URL: `https://osada.us/api/v1`. Static JSON served by Cloudflare Pages, no key needed. **Sends an `ETag`** (conditional requests work). Run by one person on a best-effort basis.
- Usage policy: search and AI input allowed, AI training not allowed. Source URLs and timestamps must be cited. We show the source and timestamp in the status bar.
- The data is scraped from the official JSA endpoints.
- Coverage: Makuuchi and Juryo, **current tournament only**. Archive pages start in 2026-03, but only the current tournament is served as JSON.
- Update schedule (documented in the project's policy file):
  - Results every **3 minutes from 15:00 to 18:59 JST** on tournament days.
  - Bout schedules at 13:00, 15:00, 17:00 and 19:00 JST.
  - Runs on GitHub Actions cron, so the timing can drift.

Endpoints used:

| Endpoint | Used for | Notes |
|---|---|---|
| `GET /torikumi.json` | **The whole current tournament in one request:** `resultDays[]` and `scheduleDays[]` for each day, Makuuchi and Juryo bout lists, `isPlayoff`, `absentees`, `status`, and the timestamps `updatedAt`, `resultUpdatedAt` and `scheduleUpdatedAt` | Bout names are **hiragana only**; `eastEnglish` is usually empty. Ranks are in Japanese (`前頭十七枚目`). `kimarite` is Japanese, and `未定` means undecided. `winner` is `east`, `west` or `null`. Wrestler identity comes from `…/profile/{nskId}/` in `eastProfileUrl`. |
| `GET /banzuke.json` | Current ranking with kanji `name`, `yomi` (reading), `id` (= JSA ID), `results[15]` (`win`, `loss`, `draw` or `null`) | **Doesn't distinguish fusen wins/losses**: a fusen loss is `loss`, and absences are `draw` (Phase 9 finding). Prefer sumo-api.com for those details once it has caught up. |
| `GET /rikishi.json`, `/rikishi/{id}.json` | Optional profile data | Not needed in v1 |

Between tournaments, osada.us keeps serving the last tournament until the new ranking is published.

### 4.3 Bundled snapshot (built from sumo-api.com)

Finished tournaments never change. Instead of every user fetching them, a separate tool fetches them once and the result ships inside the binary.

- **Tool:** `basho-data` (`cmd/basho-data`). It is a developer tool in the same module and is not shipped via Homebrew.
- **Contents** (`data/snapshot/`, embedded with `go:embed`):
  - `manifest.json`: schema version, generator version, generation time, source, list of included tournaments, a SHA-256 checksum per file.
  - `basho/YYYYMM.json.zst`, one file per finished tournament, containing:
    - metadata (dates, yusho, sansho)
    - Makuuchi and Juryo banzuke (rank, wrestler, shikona at that tournament, 15 day results)
    - every bout of both divisions (day, match number, both sides with ranks, winner, kimarite, playoff flag)
  - `rikishi.json.zst`: every wrestler who appeared in Makuuchi or Juryo since 1960, with IDs (`id`, `nskId`, `sumodbId`), names, heya, birth date, hometown, debut, latest height and weight, and retired flag.
- **Format:** compact JSON compressed with zstd (`github.com/klauspost/compress/zstd`). Each tournament file is written once and never changes, so an update only **adds** files to git. Only `rikishi.json.zst` and the manifest are rewritten.
  - Each fact is stored once: a bout side is only the wrestler's id when their shikona and rank match the banzuke (visitors from Makushita are written out), and a banzuke record is one outcome letter per day, with opponent and kimarite taken from that day's bout unless they differ. Decoding restores the data exactly.
  - Every file carries the schema version (`"v"`), and encoding is deterministic (bouts sorted by day, division, playoff, match number), so a rebuild leaves git unchanged.
  - sumo-api writes a placeholder opponent (7402) on absent days; the adapter maps it to none.
  - **Gaps in the source.** In a tournament that is final, an empty bout list is a gap, not a day still to come: the builder logs it and writes the tournament without that day, whose records stay pending (196503 day 7, 196505 day 6 and 197811 day 5, both divisions). A record day that is pending while that day's bout is decided, against the same opponent, takes the bout's result, and the builder logs it (202507 Juryo day 15, Nishikigi and Fujiseiun). Pending otherwise means no bout that day.
- **Size:** measured on the first full build (V8): 399 tournaments, 195,099 bouts, 2.50 MB for the tournament files plus 37 KB for the 997-wrestler index. Embedding stays well under the 15 MB limit, above which it would ship as a separate release asset.
- **Building it:**
  - **First full build:** about 400 tournaments × about 34 requests (1 basho, 2 banzuke, 30 torikumi) ≈ 13,200 requests at 1 request per second; it took about 3.5 hours on 2026-10-13. This runs once, not on a tournament day (D24).
  - **Updates:** the tool reads the manifest and only fetches tournaments that are finished (rule in §7.2) and not yet included. That is about 34 requests per new tournament, plus the wrestler index, which is rewritten whenever a tournament is added, including one older than the newest.
  - **Resumable:** requests go through the normal `httpx` client (§7.1) and a work cache (`.cache/basho-data/`, ignored by git), so an interrupted run continues without fetching anything twice.
  - **Verification:** `basho-data snapshot verify` checks that each wrestler's bouts match their banzuke record and totals, that every bout appears for both wrestlers, that the schema is valid and that the checksums match. The builder runs the bout-against-record checks on the fetched data before encoding (where every check is meaningful) and logs oddities without stopping. Accepted oddities in the source data go in `data/snapshot/known-issues.txt`, one exact problem line each with a `#` reason; `verify` subtracts them and flags entries that no longer occur.
- **Update flow:** a few days after each tournament, `make snapshot` (update and verify) and `make portraits` (new wrestlers and photos; the work cache in `.cache/basho-data` makes the rest free, so keep it between runs). Commit `data: snapshot through 202611`, then tag a patch release. Shipping fresh data keeps what every installed basho asks of sumo-api, the volunteer-run source, low. The app still works with a snapshot of any age, because it fetches whatever came later (§5.8).
- **Licence:** sumo-api.com publishes no terms beyond asking for responsible use. We stay within that through the limits above and credit sumo-api.com in the README, in `basho credits` and in the manifest (D24).

### 4.4 Wikimedia Commons and Wikidata (portraits, build time only)

Only `basho-data` uses these, for the optional portraits (§8.9). The app itself never contacts Wikimedia.

- **Join:** Wikidata property P6652 "Sumo Reference ID" (its URL is SumoDB's `Rikishi.aspx?r=`) equals sumo-api's `sumodbId`; P3385 "Japan Sumo Association ID" equals `nskId` and is the fallback. Property P18 points to the image on Commons.
- **Coverage:**
  - 2026-10-13: 538 Wikidata items have a JSA ID, 293 of them with an image; 35 of the 42 current Makuuchi wrestlers have an image.
  - 2026-10-15: the snapshot's index has 997 wrestlers, but only 142 with a JSA ID and 996 with a SumoDB ID. Joined by JSA ID, 88 have an image; by SumoDB ID first, 360.
- **Licences:** the samples checked are CC BY-SA 3.0 or 4.0. Only licences on an allow-list are accepted: CC0, public domain, CC BY 2.0–4.0, CC BY-SA 2.0–4.0.
- **Being polite:**
  - Follow Wikimedia's User-Agent policy (tool name and contact).
  - At most 1 request per second.
  - Fetch thumbnails only, never full-resolution originals: the 500 px one the Commons API names (`iiurlwidth=500`, one of Wikimedia's standard widths, which its caches serve; served from `thumb.wikimedia.org`).
  - Metadata comes from the Commons API, 50 files per request.

---

## 5. Architecture

### 5.1 Package layout

```
cmd/basho/main.go              entry point; wires Fang/Cobra
cmd/basho-data/main.go         developer tool: snapshot, rating calibration, portraits (not shipped)
data/                          embedded snapshot, rating parameters, portraits (go:embed)
internal/
  domain/                     pure types and logic, no I/O
    basho.go  rank.go  bout.go  rikishi.go  outcome.go  calendar.go  jst.go
  derive/                     computed views (standings, kinboshi, last-N, h2h, as-of cutoffs)
  snapshot/                   loads the embedded snapshot; bout index (§5.8)
  rating/                     Elo-style ratings and win probability (§6.8)
  portrait/                   portrait decoding and half-block rendering (§8.9)
  datatool/                   snapshot builder and verifier, rating calibration, portrait pipeline (used by basho-data only)
  source/
    source.go                 provider interfaces, capability flags, errors
    sumoapi/                  sumo-api.com adapter (HTTP client, DTOs, mapping)
    osada/                    osada.us adapter (DTOs, Japanese rank/kimarite parsing, kana→romaji)
    resolver/                 policy-based routing and fallback across providers
  idmap/                      sumo-api ID ↔ JSA nskId ↔ osada ID mapping
  httpx/                      polite HTTP: rate limiter, retries, ETag, User-Agent, circuit breaker
  cache/                      on-disk cache with data-class time-to-live (TTL) and stale-while-revalidate
  service/                    use-case façade for UI and CLI (GetDay, GetBout, GetStandings, …)
  live/                       live-window scheduler and poller
  spoiler/                    reveal horizon (how far the user has revealed results) and its persistence
  config/                     config file, environment, flags, defaults
  clock/                      Clock interface (real or fake); JST lives in domain/jst.go
  asciifold/                  ASCII stand-ins for the symbols, for --ascii in the TUI and the CLI
  tui/
    app.go                    root model, screen stack, global keys, status bar
    theme/                    Lip Gloss styles, adaptive colours, glyph sets
    picker/  day/  bout/  standings/  help/
    components/               daybar, boutrow, statrow, hoshibar, pips, sparkbar
  cli/                        subcommands, text renderers, JSON envelopes
testdata/                     recorded API fixtures (see §13)
```

Dependency rules:
- `domain` and `derive` import nothing internal.
- `tui` and `cli` only talk to `service` for data. They may use `domain` and `derive` types, which the service's views carry (`derive` does no I/O).
- Only `source/*` and `httpx` touch the network.
- `cmd/basho` never imports `datatool`.

### 5.2 Domain model (sketch)

```go
type BashoID struct{ Year int; Month time.Month } // prints/parses "YYYYMM"; validated: ≥ 196001, odd month
type Date struct{ Year int; Month time.Month; Day int } // calendar day in Japan; ==-comparable, map key, "2026-11-08"
type Division int              // Makuuchi, Juryo
type Side int                  // East, West

type RankTitle int             // Yokozuna, Ozeki, Sekiwake, Komusubi, Maegashira, Juryo, ...
type Rank struct {
    Title  RankTitle
    Number int                 // 1-based
    Side   Side                // East = 1, West = 2; zero = none
}
func (r Rank) Short() string   // "Y1e", "O2w", "M17e", "J2w"
func (r Rank) Compare(o Rank) int // highest first; East before West (rankValue cannot order sides)
// The source strings ("Maegashira 17 East", "前頭十七枚目") stay in the adapters.
// DivMakushita / Makushita exist only for Makushita wrestlers fighting in Juryo bouts.

type Outcome int               // Pending (zero), Win, Loss, FusenWin, FusenLoss, Absent, Draw, Unknown; JSON by name

type DayResult struct {        // day = index in BanzukeEntry.Results + 1
    Outcome    Outcome
    OpponentID RikishiID       // zero when absent
    Kimarite   Kimarite        // canonical romaji key, e.g. "yorikiri"
}

type BanzukeEntry struct {     // the division follows from Rank.Title.Division()
    RikishiID RikishiID
    Shikona   Name             // as of this tournament
    Rank      Rank
    Results   [15]DayResult    // index = day-1
}

type BoutSide struct { RikishiID RikishiID; Shikona Name; Rank Rank }
type Bout struct {
    Basho     BashoID
    Division  Division
    Day       int              // 1..15
    MatchNo   int              // 1-based display order
    East, West BoutSide
    Winner    Side             // zero = not decided yet
    Kimarite  Kimarite
    IsPlayoff bool
}

type Name struct { En, Jp, Kana string }   // Jp cleaned (see §5.6); Kana only from osada.us

type Rikishi struct {
    ID RikishiID; NSKID int; SumoDBID int
    Name Name; Heya string; Shusshin string
    Birth Date; Debut BashoID
    Height, Weight float64     // current, from profile
}

type Basho struct {
    ID BashoID; Name string /* "Aki" */; Venue string
    Start, End Date
    Yusho map[Division]RikishiID
    Sansho []Prize
    Status BashoStatus         // Upcoming, BanzukeOut, InProgress, Finished, Cancelled
}

type DayStatus int             // NotPublished, Scheduled, InProgress, Complete

type Provenance struct {
    Source          string     // "sumo-api", "osada", "cache:sumo-api"
    FetchedAt       time.Time
    SourceUpdatedAt *time.Time // e.g. osada resultUpdatedAt
    Stale           bool
}
```

Every value returned by `service` comes as `Result[T]{ Value T; Prov Provenance }`.

### 5.3 Provider interfaces

```go
type HistoricalSource interface {           // sumo-api
    Basho(ctx, BashoID) (Basho, error)
    Banzuke(ctx, BashoID, Division) ([]BanzukeEntry, error)
    Torikumi(ctx, BashoID, Division, day int) ([]Bout, error)
}
type RikishiSource interface {              // sumo-api
    RikishiIndex(ctx) ([]Rikishi, error)    // bulk, paged internally
    Rikishi(ctx, RikishiID) (Rikishi, error)
    Matches(ctx, RikishiID) ([]Bout, error) // full career; fallback only when no snapshot (§5.8)
    Stats(ctx, RikishiID) (CareerStats, error) // fallback only when no snapshot
}
type CurrentSource interface {              // osada (and sumo-api through an adapter)
    Current(ctx) (CurrentSnapshot, error)   // whole tournament: banzuke and all published days, both divisions
}
```

Error types: `ErrNotFound`, `ErrNotYetAvailable` (for example a future tournament or an unpublished day), `ErrUpstream` (5xx, timeout, bad JSON), `ErrRateLimited` (429), `ErrCircuitOpen`.

### 5.4 Resolver and fallback policy

The resolver chooses sources **per data type and tournament state**, and records which one answered.

| Request | Tournament finished | Tournament in progress (or ranking just published) |
|---|---|---|
| Basho metadata | sumo-api | sumo-api, then osada for the dates |
| Banzuke (ranks) | sumo-api | sumo-api, then osada |
| Day results and records | sumo-api | **osada**, then sumo-api |
| Day schedule (future days) | — | **osada**, then sumo-api |
| Profiles (incl. height, weight) | sumo-api (no fallback; fields left empty) | same |
| Matches, kinboshi, head-to-head | sumo-api (no fallback) | same, plus results from the live snapshot for today |

Rules:

1. **Fallback happens** on `ErrUpstream`, `ErrRateLimited`, `ErrCircuitOpen`, or when the primary is **behind**. Behind means the secondary's data is newer (for example more decided bouts today, or a newer `SourceUpdatedAt`).
2. **Merging during the current tournament:** start from sumo-api's banzuke because it has fusen and absence detail. Overlay osada results for any day/bout where sumo-api has `Pending` and osada has a winner. Each bout keeps its own source. If both sources have a result and they disagree, **osada wins for today and sumo-api wins for earlier days**. The disagreement is logged at debug level.
3. **Forced source:** `--source sumoapi|osada` turns off the resolver; the CLI and TUI show "source: forced".
4. **Serving stale cache:** if every source fails and the cache has any copy (even an expired one), serve it with `Stale=true` and a status-bar warning. Without a cache, show an error screen with a retry option.

### 5.5 Identity mapping (`idmap`)

- osada `id` and the ID in `eastProfileUrl` are the **JSA ID**. sumo-api's `nskId` is the **same number**. This was confirmed for Onosato: osada 4227 = sumo-api `nskId` 4227.
- The mapping is built from the bulk `RikishiIndex` and cached for 7 days. If osada mentions a JSA ID that isn't in the mapping, refresh the index **at most once per hour**.
- If a wrestler still isn't found (for example a newcomer not yet in sumo-api), create a temporary entry with:
  - `Name.Jp` from osada, `Name.Kana` from `yomi` or the bout name
  - `Name.En` **transliterated from kana** (§5.6)
  - all other fields empty

  The VS screen shows "profile unavailable" for the empty fields.

### 5.6 Names and text handling

- sumo-api `shikonaJp` contains `四股名　名` (ring name, full-width space, given name). The **display name is the part before the first U+3000 (full-width space)**. Bracketed readings such as `伊波(いなみ)` are removed for display.
- Older tournaments have an empty `shikonaJp`; show romaji only.
- For retired wrestlers, sumo-api's `/rikishis` gives `shikonaEn` as ring name and given name (`Hakuho Sho`); the adapter keeps the part before the first space, like the kanji rule above.
- Kana to romaji (only for the fallback above): modified Hepburn without macrons. Long vowels are collapsed (`おう`/`おお` → `o`, `うう` → `u`), so `おおのさと` → `Onosato`. Capitalise the first letter. This is known to be imperfect, so mark such names with a dim `≈`.
- Kimarite: one table with 82 entries plus non-technique results, mapping canonical romaji keys to romaji display, kanji, and an English gloss (for example `yorikiri` / 寄り切り / "frontal force out"). Both adapters map into these keys. Unknown values are passed through unchanged.
- Width: all layout measures display width with Lip Gloss / `ansi.StringWidth`, never `len()`, so kanji take two cells.

### 5.7 Tournament calendar (`domain/calendar.go`)

- Tournament months are fixed (1, 3, 5, 7, 9, 11).
- **Start date:** the API's `startDate` when available. Otherwise estimate the **second Sunday of the month** and label it "(est.)".
- Day *N* date = start + (N−1) days. Day 15 = senshuraku.
- **Current tournament:** the newest tournament ID ≤ the current month for which `Basho()` returns data. If today (JST) is after its end date + 1 day, the "next" tournament is `Upcoming`. That one exists in the API only once its ranking has been published (about 13 days before day 1).
- **Day status** for the current tournament:
  - `NotPublished`: no bout list yet. Days 1–2 are announced together about 2 days before the start; after that, each bout list comes out the afternoon before the day.
  - `Scheduled`: bout list exists, no results.
  - `InProgress`: today, at least one result missing, inside the live window.
  - `Complete`: every bout in the enabled divisions has a winner, and no playoff is pending on day 15.
- **Cancelled tournaments** (known: 2011-03 Haru, 2020-05 Natsu) come from a built-in list and appear in the picker as "cancelled", not selectable. 2011-05 was a "technical examination tournament" and is labelled as such. In `domain` this is `BashoID.Held()` (`HeldRegular`, `NotHeld`, `HeldSpecial`); the service maps `NotHeld` to `BashoStatus` `Cancelled`.

### 5.8 Data layers

The service asks three layers in order:

1. **Snapshot** (embedded, §4.3): every tournament listed in the manifest. These are never fetched or cached again.
2. **Disk cache** (§7.2): tournaments that finished after the snapshot was built, plus the current tournament.
3. **Network**, through the resolver (§5.4).

Consequences:

- Browsing history needs no network and works offline from the first run.
- **Derived statistics count sekitori bouts only** (Makuuchi and Juryo). This applies to head-to-head, moveset, last 5, rating, and career yusho/sansho as of a date. Kinboshi are Makuuchi-only by definition anyway.
- All of these are computed from the snapshot plus later tournaments, so `/rikishi/{id}/matches` and `/rikishi/{id}/stats` aren't needed in normal use. They remain in the adapter as a fallback when no snapshot is present (`--no-snapshot`, for development).
- A newly promoted wrestler's last 5 only shows their sekitori bouts so far, padded with `·` and labelled "new".
- An in-memory **bout index** (by wrestler and by tournament) is built from the snapshot the first time it is needed (about 50 ms, V8) and extended with later tournaments and live results: every day the service reads from the remote layers for a tournament after the snapshot goes into the index and the ratings. Tournaments after the snapshot are not fetched in full in the background (that would break the request budget, §7.3); a snapshot update covers them.
- Provenance for snapshot data reads `snapshot (2026-11-25)`, the snapshot's build date. Remote answers read `sumo-api` when any response came from the network, `cache:sumo-api` when all came from the disk cache, with the oldest response's time and `stale` when an expired entry stood in for a failed source; facts from the calendar alone (a cancelled tournament) read `calendar`. A wrapper getter under the adapter records this per call (`service.Recording`).

### 5.9 Dependencies

The rule: **standard library first.** A third-party module is only added when it saves real work and is well maintained. Anything not on this list needs an entry here first.

**Runtime (shipped in `basho`):**

| Module | Purpose |
|---|---|
| `charm.land/bubbletea/v2` | TUI framework (Elm-style model/update/view) |
| `charm.land/lipgloss/v2` | Styling, layout, colour detection, display width |
| `charm.land/bubbles/v2` | List, table, viewport, spinner, help, key bindings |
| `github.com/charmbracelet/harmonica` | Spring animations (VS slide-in) |
| `github.com/charmbracelet/fang` | Styled CLI wrapper: help, `--version`, man page, completions |
| `github.com/spf13/cobra` | Subcommands and flags (required by Fang) |
| `golang.org/x/time/rate` | Per-host token-bucket rate limiting |
| `golang.org/x/sync` | `singleflight` (duplicate suppression), `errgroup` |
| `github.com/klauspost/compress/zstd` | Snapshot compression |
| `github.com/BurntSushi/toml` | Config file |

**Standard library instead of frameworks:**

| Need | Package |
|---|---|
| HTTP client | `net/http` (wrapped by `httpx`) |
| JSON | `encoding/json` |
| Logging (debug log) | `log/slog` |
| Embedded data | `embed` |
| JST | `time.FixedZone` (UTC+9; Japan has had no DST since 1951), checked against `Asia/Tokyo` in a test |
| Hashing (cache keys, manifest checksums) | `crypto/sha256` |

**Build and test only:**

| Module or tool | Purpose | How |
|---|---|---|
| `github.com/charmbracelet/x/exp/teatest/v2` | Driving the TUI in tests | test dependency |
| `github.com/charmbracelet/x/exp/golden` | Golden-file assertions (`-update` flag) | test dependency |
| `github.com/google/go-cmp` | Readable diffs of domain structs | test dependency |
| `mvdan.cc/gofumpt` | Formatting | `tool` directive in `go.mod`, run as `go tool gofumpt` |
| `golang.org/x/vuln/cmd/govulncheck` | Vulnerability check | `tool` directive, `go tool govulncheck` |
| `golangci-lint` | Linting | Homebrew (already installed; the project advises against building it from source) |
| `goreleaser` | Release builds (Phase 17) | Homebrew; `make tools` reports if missing |

**Portraits (Phase 14):** `github.com/esimov/pigo` (face detection, `basho-data` only; scaling is our own area averaging), `github.com/charmbracelet/x/mosaic` (half-block rendering, or our own renderer), and `vhs` (Homebrew) for the README GIF.

**Charm v2 API facts and import-path rules:** `docs/charm-v2-notes.md`. Versions: `go.mod`.

**Deliberately not used:** Viper (TOML plus our own precedence rules is enough), testify (standard `testing` plus go-cmp), an HTTP client library, a dependency injection framework, a database (plain files and embedded data cover everything).

---

## 6. Derived data (`derive/`)

All inputs come from the bout index (§5.8). Every computation takes an **as-of cutoff** `AsOf{Basho; Day; Division; MatchNo; Playoff; Inclusive}`: a point in the order of play (tournament, day; within a day Juryo before Makuuchi, a division's playoffs after its regular bouts, then match number). `MatchNo` 0 means the whole day; constructors `BeforeDay`, `AfterDay`, `EndOf`, `BeforeBout`, `ThroughBout` and `NoCutoff` cover the uses. A banzuke day result without a bout (absence, source gap) counts once its day has started. This one mechanism handles three things:
- the correct record and stats going into a bout
- standings after day N
- spoiler-free mode (§9)

### 6.1 Record going into a bout

Record = the banzuke results before the cutoff (`RecordAt` with `BeforeBout`; the record after the bout uses `ThroughBout`). Going into a playoff, that includes the day's regular bout; playoffs themselves never count, as in the official record.
- Fusen wins/losses count as wins/losses.
- Absences are counted separately and shown as `W-L-A` only when A > 0, for example `5-7-3`. Draws (old era) are shown as `7-7 1d`; sumo-api records most `hikiwake` with a winner, so they rarely occur.
- Kachi-koshi is 8 wins; make-koshi is 8 losses and absences together.

### 6.2 Last 5 results

The wrestler's last 5 decided bouts **before** the cutoff, taken from their career in the bout index, **crossing tournament boundaries** (on day 2 this shows 1 result from this tournament and 4 from the previous one, separated by a thin divider `│`). Absences are skipped. Fusen is shown as □/■. With fewer than 5 sekitori bouts the result is marked new (§5.8).

The bout index is in memory from the snapshot and extended with later tournaments, so no placeholder is needed while matches load.

### 6.3 Kinboshi

Rule: count the wrestler's matches where
- the wrestler's rank title at the time was `Maegashira`,
- the opponent's rank title was `Yokozuna`,
- the wrestler won,
- `kimarite ≠ fusen` (nor a draw, which sumo-api records with a winner),
- the match was a regular bout (not a playoff) in the Makuuchi division,
- and the match was before the cutoff.

`derive.KinboshiOf` counts a career in the bout index in microseconds, so nothing is cached; `derive.IsKinboshi` also drives the VS stamp and the recap.

Display:
- `★` per kinboshi. Five or more becomes `★×12`.
- Kinboshi from **this tournament** are drawn in a brighter gold.
- Shown for every wrestler, not only current Maegashira, because a san'yaku wrestler can have kinboshi from earlier tournaments.

Also derived (shown in the day recap, §8.5): `ThisBashoScalps` (`derive.Scalps`) = regular-bout wins by technique against Yokozuna/Ozeki in this tournament, before the cutoff.

### 6.4 Head-to-head record

Computed from one wrestler's cached career matches, filtered to the opponent's ID, before the cutoff. It contains:
- wins for each side (fusen included, as in the official record)
- each side's top 2 winning techniques (fusen and draws left out; ties by name)
- the last n meetings, newest first (tournament, day, both ranks, winner, technique): 3 on the VS screen, up to 10 expanded

The API's `/matches/{b}` endpoint is not used for this, because it can't apply a cutoff.

### 6.5 Standings after day N

Inputs: banzuke entries, cut off at day N.
- **Sort order:** wins descending, losses and absences together ascending, then banzuke rank. Grouped into bands by wins behind the leader: `Leader(s)`, `−1`, `−2`, then everyone else collapsed (expandable).
- **Contention:** a wrestler is still in contention if their wins + days remaining ≥ the leader's wins (with a cutoff inside a day, a wrestler whose bout is still to come has that day remaining). Show "✓ yusho" once a sole leader's wins are more than anyone else's maximum possible wins. When the top is shared and nobody in contention has a day left, the yusho goes to a playoff (`Playoff`); sumo-api's bout lists carry no playoffs, so for finished tournaments the winner comes from the tournament's yusho (202609 Juryo: three at 11-4).
- **Markers:**
  - 8+ wins: kachi-koshi, green, `KK`
  - 8+ losses and absences together: make-koshi, red, `MK`
  - kachi-koshi or make-koshi reached **on day N** (the latest day begun at the cutoff): bold "new" marker
  - absent on the latest day counted: `休` (kyujo)
- **Hoshitori grid:** one row per wrestler in banzuke order, 15 result columns, with day N's column highlighted.

### 6.6 Career statistics as of a date

- **Height and weight:** sumo-api only records them at a wrestler's debut, so per-tournament values do not exist. The wrestler index holds the latest values (updated over a career; for retired wrestlers the last known), and the VS screen uses those, labelled "latest" for tournaments other than the current one.
- **Age:** birth date to tournament start date.
- **Yusho and sansho:** counted exactly as of the tournament, from each tournament's yusho (Makuuchi and Juryo) and special prizes (by type) in the snapshot plus later tournaments (§5.8). A tournament counts once all of it, playoffs included, lies before the cutoff (`EndOf` or later), so nothing is spoiled.
- **Heya and hometown:** sumo-api only provides current values. Stable closures and moves aren't reflected (known limitation, noted in the help screen).
- **Shikona:** the name from the bout or ranking is as of that tournament. If it differs from today's name, show "now: X" dimmed.

---

### 6.7 Technique profile ("moveset")

Built entirely from the cached career matches (§6.2–6.4), so it needs **no extra requests**.

**Categories.** Every kimarite in the table (§5.6) gets one fan-oriented category. The table in `domain/kimarite` is authoritative; examples:

| Category | Meaning | Examples |
|---|---|---|
| PUSH | Pushing and thrusting (oshi/tsuki), including from behind | oshidashi, oshitaoshi, tsukidashi, tsukitaoshi, okuridashi, okuritaoshi |
| BELT | Belt fighting (yotsu), forcing out or down | yorikiri, yoritaoshi, abisetaoshi, kimedashi, tsuridashi |
| THROW | Throws, twists, trips, backward body drops | uwatenage, shitatenage, sukuinage, kotenage, uwatedashinage, tsukiotoshi, makiotoshi, sotogake, ketaguri, utchari |
| PULL | Pulling and slapping down | hatakikomi, hikiotoshi, hikkake, katasukashi |
| OTHER | Rare techniques and the opponent's own mistakes | isamiashi, koshikudake, tsukite, fumidashi, hansoku |

Fusen wins and losses are **excluded** from every profile.

**Sample window.**
- Default: the wrestler's **last 6 tournaments** with sekitori bouts before the as-of cutoff (§6), including earlier days of the current tournament.
- If the window has fewer than 20 wins, it is widened one tournament at a time, up to the whole career.
- Other choices: `career`, `this basho`.
- The window and sample size are always shown ("last 6 basho · 61 W").

**Output per wrestler:**
- Win share per category and the top 3 winning techniques (count).
- Loss share per category and the top 3 techniques the wrestler lost to (shown as "Loses to").
- Signature move: the most frequent winning technique and its share of all wins.
- **Fighter type**, first rule that matches:
  1. PUSH ≥ 50% of wins → "Pusher-thruster"
  2. BELT + THROW ≥ 55% → "Belt fighter"; if THROW alone is ≥ 30%, "Thrower"
  3. PULL ≥ 25% → "Counter-puller"
  4. otherwise → "All-rounder"

  Tuned in Phase 4 on career profiles of about 45 well-known wrestlers; only the thrower share moved, from 30% to **35%**, so that classic yotsu wrestlers such as Taiho (33% THROW) and Takanohana (32%) stay belt fighters. Reference results, checked in a test: Takakeisho, Abi, Chiyotaikai, Tamawashi pusher-thrusters; Kotoshogiku, Terunofuji, Tochinoshin, Kyokutenho, Taiho belt fighters; Hakuho, Chiyonofuji, Hoshoryu, Enho throwers; Tobizaru counter-puller; Ura, Mitakeumi, Takayasu all-rounders.
- **Matchup line:** A's main winning category against the share of B's losses in that category, and the same the other way round. Show the direction with the larger product.

**Spoilers:** the as-of cutoff applies, so the profile only uses bouts before the bout being viewed (and before the reveal horizon in spoiler-free mode).

**CLI:** `basho bout` prints both profiles under the tale of the tape. `--json` includes them as `moveset` objects.

### 6.8 Rating and win chance

There is no public sumo rating API. Ozeki Analytics published an Elo analysis covering 1958 to today as blog posts (no data feed), which is useful as a plausibility check. We compute our own rating.

**Model: Elo**, centred on 1500, processed bout by bout in date order over all sekitori bouts.

- Win chance for A against B: `1 / (1 + 10^((R_B − R_A) / 400))`.
- Fusen bouts and draws are skipped. Playoff bouts count.
- **Starting rating:** lower-division bouts aren't available. So a wrestler's first sekitori bout starts at the average rating of established wrestlers (those with earlier rated bouts, as they entered each tournament) in the same rank band over the previous 6 tournaments.
  - Bands: each san'yaku title, M1–5, M6–10, M11–17, J1–7, J8–14, and Makushita for visitors in Juryo bouts.
  - During the 1960 warm-up period, and whenever the window holds fewer than 10 established wrestlers in the band, a fixed table by rank is used (Appendix F).
- **K factor:** a higher value (`K_new`) for a wrestler's first 30 sekitori bouts, then `K`.
- **Return after absence:** for each tournament held but missed completely (fully absent, or ranked below Juryo), the rating is pulled a fraction `r` towards the rank-band average at the return: `avg + (R − avg)·(1 − r)^missed`. As-of queries before the return already include the pull; `NoCutoff` gives the latest rating unpulled.
- The parameters `K_new`, `K` and `r` are stored in `data/rating/params.json` (embedded) and come from calibration.

**Calibration** (`basho-data rating calibrate`):

- Grid search over the parameters. The parameters are **chosen by log loss on the training years** 1965–2014 (1960–64 are the warm-up and not scored) and **judged on the evaluation years**, 2015 to the newest tournament in the snapshot. Choosing on the evaluation years, as first planned, would score the model on the data that picked it.
- Metrics: log loss, Brier score, accuracy, and a calibration table (predicted vs actual in 10% buckets).
- Baselines: 50/50, and "higher rank wins" with a probability fitted by rank difference.
- **The rating is only shown if it beats the rank baseline on log loss** (V9). Otherwise the feature stays hidden and the model is revisited (for example Glicko-2, or a blend with rank).

**At runtime:**

- Ratings are recomputed from the bout index the first time they are needed (about 195,000 bouts, about 40 ms on an Apple M5) and continue through later tournaments and live results: the newest tournament can be set again, replayed from a checkpoint taken before it.
- Everything respects the as-of cutoff, so the VS screen always shows ratings from *before* the bout, and spoiler-free mode stays intact.

**Display:**

- **VS screen:** row `RATING` (value plus change since the start of the tournament, `▲12`/`▼8`) and row `WIN CHANCE` (mirrored bar with percentages). **Shown by default** (D26); `ui.win_chance = false` hides both rows.
- **Day view:** optional column with the favourite's win chance, toggled with `p` (config `ui.win_chance_column`, default off).
- **Day recap:** an "upset" means the winner's win chance before the bout was below 35%.
- **CLI:** `basho bout` and `--json` include ratings and win chance. `basho ratings [-t] [--top N]` lists the top-rated wrestlers as of a tournament.
- The help screen explains that this is an estimate from past results, not betting advice.

---

## 7. Caching and being a good citizen

### 7.1 HTTP client (`httpx`)

- `User-Agent: basho/<version> (+https://github.com/thorstenpfister/basho)`
- Request gzip compression. Timeouts: 5 s to connect, 15 s total.
- **Rate limit per host** (token bucket, `golang.org/x/time/rate`):

  | Host | Rate | Burst | Max concurrent |
  |---|---|---|---|
  | sumo-api | 1 per second | 3 | 2 |
  | osada | 1 per 20 s | 1 | 1 |

  Configurable, but **only to stricter values**: the built-in values are the ceiling.
- **Retries:** only for timeouts, connection errors and 5xx. At most 2, with exponential backoff (1 s, 3 s) ± 20% jitter. Never retry 4xx except 429.
- **429:** respect `Retry-After` (default 60 s). Open the per-host circuit for that long, and use the fallback source in the meantime.
- **Circuit breaker:** after 3 consecutive failures, the host is "open" for 2 minutes, then one trial request ("half-open").
- **Conditional requests:** send `If-None-Match` and `If-Modified-Since` when a stored validator exists. A 304 refreshes `FetchedAt` without a new body.
- **Duplicate suppression:** identical requests in flight at the same time are merged (`singleflight`).
- Debug log of every request (`--debug`, written to `$XDG_STATE_HOME/basho/debug.log`): method, URL, status, bytes, duration, cache hit/miss.

### 7.2 On-disk cache (`cache`)

- Location: `$XDG_CACHE_HOME/basho/`, otherwise `~/Library/Caches/basho` on macOS. One file per request key. The key is a hash of source and URL; a human-readable path index makes debugging easier.
- Each entry is one file: a JSON header line (key, `FetchedAt`, `ETag`, `Last-Modified`, pinned flag, schema version) and the gzipped body. A 304 only rewrites the header (`Touch`).
- **Freshness is decided at read time by the caching layer**, not stored: the same response changes class as the calendar moves (today's bouts are "live" now and "earlier day" tomorrow; the current tournament becomes finished). The cache only knows which entries are pinned (finished tournaments, never evicted).
- Freshness is decided by data class:

  | Data class | Fresh for (TTL) | Notes |
  |---|---|---|
  | Finished tournament: metadata, banzuke, bout lists | **forever** | Finished = end date + 2 days has passed **and** a yusho winner exists. The 2-day margin allows for late corrections. |
  | Current tournament: earlier days | 30 min | Fixes do happen |
  | Current tournament: today, during the live window | the live interval (§10) | |
  | Current tournament: anything, outside the live window | 60 min | |
  | osada `torikumi.json` / `banzuke.json` | as for the current tournament | Conditional GET makes refreshes cheap |
  | Wrestler bulk index / ID mapping | 7 days | Forced refresh at most once per hour when an unknown ID appears |
  | Career matches per wrestler | until the next JST day boundary, or the end of the live window | Topped up incrementally |
  | Career stats | 24 h, or until the current tournament ends | |
  | "Not found" answers (future tournament) | 6 h | Avoids repeated requests while browsing |

- **Stale-while-revalidate:** cached data (even expired) is shown immediately, refreshed in the background, and the screen updates when the response arrives. The status bar shows `↻` while refreshing.
- **Size:** an LRU cap of 200 MB removes the least recently used non-permanent entries first. A full 1960–2026 browse is estimated at about 60 MB.
- **Schema version change:** entries with an old version are ignored and overwritten.

### 7.3 Request budget (targets, checked in tests with a counting transport)

| Action | Cold cache | Warm cache |
|---|---|---|
| Open any tournament in the snapshot | **0** | 0 |
| Open a tournament that finished after the snapshot, day 1, one division | 3 requests: basho, banzuke, torikumi | 0 |
| Move to another day (not in the snapshot) | 1 | 0 |
| Toggle division (not in the snapshot) | 2 (banzuke and torikumi) | 0 |
| Open a VS screen | **0** (all statistics come from the bout index); ≤ 4 without a snapshot | 0 |
| Wrestler index | 1 request (active wrestlers) in the background, only when the snapshot's index is older than 7 days | 0 |
| Prefetch | **none by default**. Optional (`prefetch.adjacent_days = true`): the next day only, and only when idle for more than 2 s. | |
| Live mode, per poll | 1 conditional GET to osada (usually 304 or a small body) | |
| Live mode, per tournament day | ≈ 1 request every 3 min over ≈ 4.5 h ≈ **≤ 90 requests**, mostly 304; simulated: 73 (Phase 10) | |

---

## 8. TUI specification

### 8.1 General

- Bubble Tea v2 (`charm.land/bubbletea/v2`), Lip Gloss v2, Bubbles v2. Full-screen ("alt screen") mode. Mouse wheel scrolling is optional (config setting, on by default).
- The **root model** holds:
  - a screen stack, global key handling, the status bar and the help overlay
  - a shared context: service, config, theme, clock, spoiler horizon, live state
- **All I/O runs in `tea.Cmd`s**; `Update` never blocks. Each screen has `loading`, `ready`, `error` and `empty` states with a spinner or skeleton.
- **Responsive sizes** (from `WindowSizeMsg`):

  | Width | Behaviour |
  |---|---|
  | < 60 cols or < 20 rows | "Terminal too small (need 60×20)" |
  | 60–89 (compact) | Single column, short ranks (`M17e`), kanji hidden on the day view, VS screen without portraits |
  | 90–129 (normal) | Full layouts as drawn below |
  | ≥ 130 (wide) | Day view gets a standings side panel; VS screen gets the head-to-head history column |

- **Glyph sets:**

  | | Win | Loss | Fusen win | Fusen loss | Absent | Pending | Kinboshi |
  |---|---|---|---|---|---|---|---|
  | Unicode (default) | ○ | ● | □ | ■ | – | · | ★ |
  | ASCII (`--ascii`) | o | x | O | X | - | . | * |

- **Colours:** adaptive light/dark using v2 background detection. Supports `NO_COLOR` and `--no-color`.
  - Palette: neutral foreground, dim secondary text, one accent (indigo), **gold** (kinboshi, yusho), **green** (winning record, win), **red** (losing record, loss).
  - East and West are told apart by position, not colour.

### 8.2 Status bar (bottom, every screen)

```
 Aki 2026 · Makuuchi · Day 13 ─────────── ● LIVE next 2:05 · osada.us 16:42 JST (1m ago) · ? help
```

- Left: tournament, division, day.
- Right, one of:
  - live state (§10.3)
  - `source · updated <time> (<age>)`
  - `↻` while refreshing
  - `⚠ stale (offline)`
  - `🙈 spoiler-free` (ASCII: `[SF]`)
- Short-lived messages appear on the left for 3 s and then disappear, for example "No bouts published for day 14 yet".

### 8.3 Basho picker

```
BASHO ─ select tournament                                            [/] filter
 ┌──────────────────────────────────────────────────────────────────────────┐
 │ ▶ 2026  Kyushu  11  upcoming · banzuke 2026-10-26                         │
 │   2026  Aki     09  Yusho: Onosato (12-3)                                 │
 │   2026  Nagoya  07  Yusho: …                                              │
 │   2026  Natsu   05  …                                                     │
 │   ── 2025 ──                                                              │
 │   …                                                                       │
 │   2020  Natsu   05  cancelled                                             │
 └──────────────────────────────────────────────────────────────────────────┘
  enter open · / filter (year, name) · g/G top/bottom · q quit
```

- Built locally from the calendar: `196001` to the current or upcoming tournament. **No request per row.**
- Yusho text is filled in only from the snapshot and the cache (finished tournaments already viewed), with the winner's record. Otherwise the row is left empty. Browsing the list never triggers requests.
- In spoiler-free mode the yusho column is hidden for the current and most recent tournament.
- The app starts directly in the day view of the current tournament on its current or latest day. `esc` from there opens the picker.

### 8.4 Day view

```
 Aki Basho 2026 · Ryogoku Kokugikan · Makuuchi                    [d] Juryo
  1  2  3  4  5  6  7  8  9  10  11  12 [13] 14  15
  ✓  ✓  ✓  ✓  ✓  ✓  ✓  ✓  ✓  ✓   ✓   ✓   ●   ·
 Day 13 · Fri 2026-09-25                                         19 bouts
 ─────────────────────────────────────────────────────────────────────────
  #  East                         Result                       West
  1  M17e Shonannoumi   6-6  ●  oshidashi  ○  6-6  Kazuma         J2w
  2  M16w Chiyoshoma    5-7  ○  yorikiri   ●  7-5  Tokihayate    M15e
  …
▶18  O2e  Aonishiki     9-3  ○  uwatenage  ●  8-4  Kirishima     O1w
 19  Y1e  Onosato      10-2  ·      —      ·  9-3  Hoshoryu      Y1w   結び
 ─────────────────────────────────────────────────────────────────────────
 Kyujo: Wakatakakage (M12e), Wakanosho (M16e)
 [enter] VS · [s] standings · [r] recap · ←/→ day · j/k bout · L live · ? help
```

- **Day bar marks:** `✓` complete, `●` in progress, `·` scheduled, blank = not published. The selected day is in brackets and highlighted. Days with no data can't be selected and show a message instead.
- **Bout rows:**
  - The winner's name is bold, the loser's is dimmed.
  - The result glyphs sit next to the technique.
  - Records are **as of before the bout**; the record after the bout is shown on the VS screen.
  - Juryo wrestlers appearing in Makuuchi bouts keep their `J` rank.
  - The last bout of the day gets a `結び` tag. Playoff bouts are grouped below a `── Kettei-sen ──` divider; only osada has them, so they appear for the current tournament only (V2).
- **Live updates:** newly decided bouts briefly show a `NEW` badge until the next poll or 60 s. The selection never moves when the list updates.
- **Wide layout:** right-hand panel with the yusho race after the selected day (top two bands).
- **Kyujo footer:** wrestlers absent on that day (osada `absentees`, or sumo-api records with `absent`).
- **Day recap (`r`)**, a short "between rounds" summary of the day:
  - leaders after the day
  - who reached kachi-koshi or make-koshi today
  - kinboshi earned today
  - upsets (the winner's win chance before the bout was below 35%, §6.8; `service.Ratings.IsUpset`). While the rating is hidden (V9), an upset is a win by technique over a wrestler at least 6 rank levels higher, where each san'yaku title is three levels and each Maegashira or Juryo number one (`derive.IsUpset`)
  - technique of the day (the technique of the day's bouts that is rarest over the bout index; ties go to the earlier bout)

### 8.5 VS screen ("tale of the tape")

```
 Aki 2026 · Day 13 · Bout 19/19 · MUSUBI-NO-ICHIBAN
   ONOSATO                      VS                      HOSHORYU
   大の里                                                    豊昇龍
   Pusher-thruster                                   Belt fighter
   ────────────────────────────────────────────────────────────────
   Yokozuna 1 East             RANK              Yokozuna 1 West
   Nishonoseki                 HEYA                    Tatsunami
   Ishikawa                    FROM                     Mongolia
   26                           AGE                           27
   10-2                       RECORD                         9-3
   ○○●○│○                     LAST 5                       ○●○○●
   190cm                      HEIGHT                     188cm
   188kg                      WEIGHT                     150kg
   ★★                        KINBOSHI                          —
   4                           YUSHO                           2
   ▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒░░▚▚        STYLE        ▓▓▓▓▒▒▒▒▒▒▒▒░░░░░░▚
   oshidashi 36%            SIGNATURE            yorikiri 31%
   1846 ▲12                   RATING                 1791 ▼8
   ━━━━━━━━━━━━━━━━━━━ 58%  WIN CHANCE  42% ━━━━━━━━━━━━━━
   ────────────────────────────────────────────────────────────────
   H2H  10 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━░░░  1
        hatakikomi ×6 · oshidashi ×3        yorikiri ×1
 ╭────────────────────────────────────────────────────────────────╮
 │              ONOSATO wins by TSUKIDASHI  突き出し                │
 │              11-2                       9-4                      │
 ╰────────────────────────────────────────────────────────────────╯
   [ ] prev/next · space reveal · v head-to-head · m moveset · esc back
```

(The values in the mockup are illustrative. In the STYLE bars, each segment has its own colour and texture. See §6.7.)

- Layout: mirrored columns around a centre column of labels. The left column is right-aligned towards the centre and the right column left-aligned (both "face" the middle).
- **Height and weight:** the latest values (§6.6), as text. The bars that once scaled them to the division's range were dropped on 2026-10-15, together with the portraits' move above the names: they drew the eye away from the comparison.
- **Result panel:**
  - Normal mode: visible when the bout is decided.
  - Spoiler-free mode: shows `[space] reveal` instead.
  - Undecided bout: `Scheduled — Day 13`, or in live mode `Waiting for result…`.
  - Content: winner, technique in romaji and kanji with its English gloss on a third line, records after the bout. A kinboshi bout gets a gold `★ KINBOSHI` stamp; a fusen bout gets `FUSEN (default)`.
- **Head-to-head history (`v`):** expands the last 3 to 10 meetings (tournament, day, ranks, winner, technique). In the wide layout it is a side column and always visible.
- **RATING and WIN CHANCE** (§6.8): ratings before the bout. The win-chance bar is drawn from the centre outwards on both sides, so the favourite's bar is longer. Hidden if the rating didn't pass calibration (V9).
- **Fighter type and STYLE:**
  - The fighter type (§6.7) is shown dimmed under the kanji name, like a character class.
  - The STYLE row is a 20-cell stacked bar per wrestler, coloured and textured by category (█ ▓ ▒ ░ ▚; letters P B T L O without colours or in ASCII), in a fixed order (PUSH, BELT, THROW, PULL, OTHER). On the left side the order is reversed so that PUSH sits next to the centre label on both sides.
  - SIGNATURE shows the most frequent winning technique and its share.
  - When fewer than 10 wins are in the sample, both rows show `— not enough data —`.
- **Moveset panel (`m`):** an overlay with the full technique profile for both wrestlers side by side:

  ```
   MOVESET · last 6 basho                                            [m] close
     ONOSATO   61 W · 19 L                   HOSHORYU   48 W · 21 L
     Pusher-thruster                         Belt fighter
     PUSH   ████████████░░░░░░░░  58%        PUSH   ████░░░░░░░░░░░░░░░░  19%
     BELT   ████░░░░░░░░░░░░░░░░  21%        BELT   ████████░░░░░░░░░░░░  38%
     THROW  ██░░░░░░░░░░░░░░░░░░  10%        THROW  ███████░░░░░░░░░░░░░  33%
     PULL   ██░░░░░░░░░░░░░░░░░░  10%        PULL   ██░░░░░░░░░░░░░░░░░░   8%
     OTHER  ░░░░░░░░░░░░░░░░░░░░   1%        OTHER  ░░░░░░░░░░░░░░░░░░░░   2%
     Wins by    oshidashi 22 · hatakikomi 9  Wins by    yorikiri 15 · uwatenage 9
                yorikiri 8                              oshidashi 7
     Loses to   yorikiri 7 · uwatenage 4     Loses to   oshidashi 6 · hatakikomi 4
   ─────────────────────────────────────────────────────────────────────────
     Matchup: PUSH vs BELT · Onosato wins 58% by PUSH; Hoshoryu loses 29% to PUSH
     Window: [w] last 6 basho ▸ career ▸ this basho
  ```

  - `w` cycles through the sample window (last 6 tournaments → career → this tournament).
  - The matchup line compares one wrestler's main winning category with the share of the other wrestler's losses in that category, in both directions. The more striking direction is shown.
- **Animation (Harmonica):**
  - When the screen opens, the two name blocks slide in from the edges (spring animation, about 250 ms), and `VS` fades in from dim to bright.
  - On reveal, the result panel flashes once (inverted, about 150 ms).
  - Turned off by `ui.animations = false`, by compact layout, and when the output isn't a terminal (TTY).
- `[` and `]` move to the previous/next bout in day order, crossing into the next or previous day at the ends.
- Data loads progressively. The tale of the tape draws immediately from banzuke and bout data (rank, record, this-tournament results). Height/weight, kinboshi, head-to-head and yusho fill in as their requests finish, with `…` placeholders until then.

### 8.6 Standings view

Two tabs (`tab` switches between them):

**A. Yusho race**

```
 Standings after Day 13 · Makuuchi                              [tab] grid
  11-2   Onosato Y1e  ○○○●○○○○○○○○·    Aonishiki O2e  ○○●○○○○○●○○○·
  10-3   Kotozakura O1e …
   9-4   … (4)
 ─ out of contention ─ 8-5 and below ▸ (enter to expand)
 Kachi-koshi: 14 · Make-koshi: 11 · Kyujo: 3       Yusho decided: no
```

**B. Hoshitori grid**

```
              1 2 3 4 5 6 7 8 9 10 11 12 13 14 15   W-L-A
 Y1e Onosato  ○ ○ ○ ● ○ ○ ○ ○ ○ ○  ○  ○ [○] ·  ·   11-2
 Y1w Hoshoryu – – – – – – – – – –  –  –  –  –  –    0-0-15  休
 O1e …
```

- Rows follow banzuke order, East then West within each rank level. Kachi-koshi/make-koshi colouring applies to the W-L column. The selected day's column is highlighted, and later days are dimmed (`·`).
- `←`/`→` change the "after day N" cutoff while on this screen. `enter` on a row opens a small popup with that wrestler's results (opponent and technique for each day).

### 8.7 Help overlay (`?`)

Pages, switched with `←`/`→` or `tab` and scrolled with `j`/`k` or the wheel when longer than the screen:

- **Keys:** the screen's keys, then the global ones (Bubbles `help` in full mode).
- **Legend:** the glyphs and abbreviations, the STYLE bar's categories, `≈` names and the status bar's marks.
- **Rating:** what the rating and win chance are, that they never use the bout shown or anything after it, and how they did in calibration (from `params.json`).
- **Data:** the sources and their notes (osada's AI-use policy; sumo-api is run by a volunteer), shared with `basho credits`.
- **Limitations:** today's heya and hometown, latest height and weight, sekitori bouts only, early ratings, live delay, transliterated names, the three days missing from sumo-api.

### 8.8 Keybindings

| Key | Context | Action |
|---|---|---|
| `←`/`h`, `→`/`l` | day view, standings | previous/next day |
| `1`–`9`, `0` (=10), `!` `@` `#` `$` `%` (=11–15) | day view | jump to day; every day is also reachable as `g` and two digits: `g13`, `g05` |
| `↑`/`k`, `↓`/`j` | lists | move |
| `enter` | day view | open VS screen |
| `[` / `]` | VS screen | previous/next bout |
| `space` | VS screen | reveal result (spoiler-free mode) |
| `v` | VS screen | head-to-head history |
| `1` / `2` | VS screen | the East / West wrestler's career (wrestler screen) |
| `i` | standings grid and its popup | the selected wrestler's career |
| `enter` | wrestler screen | that tournament's day view, on the wrestler's last bout of it |
| `g` / `G` | wrestler screen | newest / oldest tournament |
| `p` | day view | toggle the win-chance column |
| `m` | VS screen | moveset panel |
| `w` | moveset panel | cycle the sample window |
| `s` | day view | standings after this day |
| `r` | day view | day recap |
| `tab` | standings | switch race/grid |
| `d` | anywhere | switch Makuuchi ⇄ Juryo |
| `L` | anywhere | toggle live mode |
| `R` | anywhere | manual refresh (30 s cooldown per resource) |
| `S` | anywhere | toggle spoiler-free mode |
| `t` | anywhere | jump to today (current tournament) |
| `b` / `esc` | anywhere | back; picker from the day view |
| `?` | anywhere | help; inside it `←`/`→` or `tab` switch pages, `j`/`k` scroll |
| `q`, `ctrl+c` | anywhere | quit (from the VS screen `q` goes back one level; `ctrl+c` always quits) |

*Note:* `h`/`l` always mean left/right (previous/next) on every screen; head-to-head uses `v` (D16).

---

### 8.9 Portraits (Phase 14)

Pixel-art portraits above the wrestlers' names, facing each other like a fighting-game character select screen (the mockup below shows the first plan, beside the tape):

```
  ▗▄▄▄▄▖                                                        ▗▄▄▄▄▖
 ▐▓▓▓▓▓▓▌  ONOSATO                VS                HOSHORYU   ▐▓▓▓▓▓▓▌
 ▐▓▀██▀▓▌  大の里                                       豊昇龍   ▐▓▀██▀▓▌
 ▐▓▓▄▄▓▓▌  Pusher-thruster                       Belt fighter   ▐▓▓▄▄▓▓▌
  ▜████▛   ──────────────────────────────────────────────────    ▜████▛
 (32×32 px = 32 columns × 16 rows in the real UI; this is only a stand-in)
```

**Build pipeline** (`basho-data portraits update`, runs together with the snapshot):

1. Query Wikidata for wrestlers in the snapshot's index with P6652 or P3385, and P18 (§4.4). Fetch the licence, author and source URL from the Commons API and drop anything not on the allow-list.
2. Fetch a 500 px thumbnail.
3. Crop to head and shoulders. Face detection uses `github.com/esimov/pigo` (pure Go) and crops a square about 1.7× the face box. Manual overrides in `data/portraits/overrides.toml` can set the crop box, exclude an image or pick a different file.
4. Downscale with area averaging to 32×32 and 20×20, then reduce each portrait to a palette of its own 16 colours (median cut in CIELAB on the 32 px image, shared by both sizes), with optional ordered dithering.
5. Store as paletted PNG (about 1.7 KB per wrestler for the three sizes, about 610 KB for 360) in `data/portraits/`, together with `credits.json` and a generated `CREDITS.md` (file, author, licence, source URL, and the changes made: "cropped, downscaled, colour-reduced").

**Licensing:** the derived images are adaptations, so they keep the source licence (mostly CC BY-SA). `data/portraits/LICENSE` says so. Attribution is shown by `basho credits`, on the help overlay's Data page, and in the README.

**Rendering:**

- Half-block characters (`▀`): the foreground colour is the upper pixel, the background colour the lower one. This is pure text, so it fits into Bubble Tea and Lip Gloss layouts and works in any truecolor terminal (Ghostty included). `github.com/charmbracelet/x/mosaic` (experimental) or a small renderer of our own.
- The West portrait is mirrored horizontally so the fighters face each other.
- On 256-colour terminals, colours map to the nearest palette entry. Portraits are hidden with `NO_COLOR`, `--ascii`, in compact layout and with `ui.portraits = false`.
- Placement and size: above the names, at 20 px, each portrait lined up with its wrestler's values next to the labels: East's right-aligned, West's left-aligned, the caption too. With the names, they keep a clear line down the middle of the screen. They show when 11 rows are free (10 and the caption) and at least one of the two wrestlers has a photo; two silhouettes alone are just a distraction. The rest of the screen takes 26 to 28 rows, so a portrait fits from about 40 terminal rows. The size is chosen once the bout's tale has loaded: while it loads the screen is a row or two shorter, and a portrait chosen then could vanish when the tale arrives. The 32 px images are for the wrestler screen (Phase 15).
  - History: first beside the tape, with a 48 px size and rules by width, they took width from the tape and the head-to-head column, left a gap between 130 and 164 columns, and distracted from the comparison. On 2026-10-15 they moved above the names, first centred at up to 32 px, then lined up with the columns at 20 px after a side-by-side comparison of screenshots: centred they looked noisy, and 32 px still pulled the eye.
- Below each portrait a dim caption gives the photo year ("photo 2014"), because some photos are old.
- **No photo:** a drawn silhouette, a head with a topknot over broad shoulders, in the rules' colour. It is clearly not a likeness.
- Full-resolution images via terminal graphics protocols (Kitty, iTerm2, Sixel) are not used: they conflict with Bubble Tea's renderer (§15).

---

### 8.10 Wrestler screen (Phase 15)

A wrestler's career as seen from one tournament, the one it opens from: every sekitori tournament up to that tournament's end, or up to its spoiler horizon in spoiler-free mode, so nothing later is told.

**Screen** (`tui/wrestler`), top to bottom:
- **Header:** the 32 px portrait at the left, when there is a photo, colours and no `--ascii`. Beside it, the name (romaji and kanji by `ui.names`) over one labelled row per fact, as on the VS screen: RANK, HEYA, FROM, AGE, HEIGHT, WEIGHT, YUSHO by division, PRIZES, KINBOSHI, RATING with its peak, STYLE (the career's bar and fighter type) and SIGNATURE. In spoiler-free mode a last row reads "as of Day 13 · Makuuchi bout 5 (spoiler-free)". The rows fill the portrait's height; the first design, two long lines of text, left most of it empty and read badly.
- **Trends:** RANK and RATING as sparklines over the most recent tournaments that fit, with first → last and "last 89 of 105" when cut.
- **Table:** one tournament a row, newest first: tournament, rank, record coloured by kachi-koshi or make-koshi ("…" while partial), rating, YUSHO, special prizes, and the shikona of the time when it differs from today's. `j`/`k` and the wheel move the cursor; `g`/`G` jump to the newest and oldest; `enter` opens that tournament's day view in the wrestler's division, on the day of their last bout.

It opens with `1`/`2` on the VS screen, as of the bout's tournament, and with `i` in the standings grid, as of that tournament. The status bar reads "Onosato · career through Aki 2026".

**Data** (`service.Wrestler`):
- **Profile:** heya, hometown, age, height and weight; today's shikona and the last rank.
- **Each tournament:** rank, the shikona of the time, record, yusho and special prizes, and the rating after it. The yusho and prizes only show once the cutoff has passed the whole tournament; the tournament seen from is partial while it runs or behind a horizon.
- **Totals:** yusho and special prizes by kind, kinboshi, and the technique profile for the career, the last 6 tournaments and the tournament seen from.

**Where it comes from:**
- **The snapshot's tournaments:** placements kept with the bout index: per wrestler and tournament, the rank, the shikona and the 15 day outcomes, collected in the same pass. That is about 3 MB and 2 ms of the start.
- **Later tournaments:** their banzuke from the remote layers.
- **Records:** counted from the day outcomes up to the cutoff (`derive.RecordThrough`): a day with a bout counts when the cutoff includes the bout, a day without one once the cutoff has reached it.
- **Ratings:** the rating after each tournament, or at the cutoff.

Records follow the source. For the two days sumo-api lacks (Haru 1965 day 7, Natsu 1965 day 6), Taiho's record is a win and a loss short of the official one, and the source counts some withdrawals as more absences than the official total does.

## 9. Spoiler-free mode

- **Reveal horizon** per tournament and division: `AsOf{Day, MatchNo}` = the last bout the user has seen.
  - Stored in `$XDG_STATE_HOME/basho/state.json`.
  - New tournaments start at `Day 0`.
- When spoiler-free mode is active:
  - Everything is computed with `derive` cut off at the horizon: records, last 5, kinboshi, head-to-head, standings, recap, picker yusho, career yusho.
  - Bouts after the horizon show names and ranks, but the result is `?` and records are frozen.
  - Standings and recap show a banner: "as of Day 12 · bout 14 (spoiler-free)". They can't be moved past the horizon.
  - **Reveal** (`space` on the VS screen) moves the horizon to that bout if it is later. Walking forward with `]` + `space` is the intended "watching along" flow.
  - Extra key in the day view, `ctrl+r`: "reveal up to here", with a confirmation prompt.
  - Live mode still fetches data, but new results never appear in the UI; the status bar only shows "3 new results hidden".
  - CLI: `--no-spoilers` applies the stored horizon; `--reveal-to 13:19` sets it.
- Configuration: `spoilers.mode = "show" | "hide"` (default `show`). `S` toggles it for the session.

---

## 10. Live mode

### 10.1 When it runs

- Only for the **current tournament** while it is `InProgress`.
- Live window (JST, configurable): **14:00–18:45**.
  - Juryo bouts start around 14:30.
  - The last Makuuchi bout usually ends around 18:00.
  - On day 15 a playoff can extend that, so day 15 runs until 19:15.
- Auto-start: `live.auto = true` (default) turns live mode on when the app starts inside a window, or when the window opens while the app is running.

### 10.2 Polling behaviour

- Each tick does one `Current()` call through the resolver: osada with `If-None-Match`, then sumo-api `Torikumi(today)` for both divisions as fallback.
- **Interval:** default 3 min, matching osada's cron. Enforced minimum **2 min**. Plus ±15 s jitter so we don't land on exact minutes.
- If the source's update timestamp hasn't changed for 2 polls in a row, the interval doubles (maximum 10 min). It goes back to the base interval on any change.
- **Stop conditions:**
  - All of today's bouts in **both** divisions are decided and no playoff is pending: state becomes "Done for today". One final check runs 30 min later to catch corrections.
  - The window ends.
- **Errors:** backoff of 2, 4, 8 and then 10 minutes; the status bar shows the error. The fallback source is tried on the same tick.
- Manual refresh (`R`) resets the timer, but respects the 30 s cooldown and the rate limiter.
- Implementation: a `live.Scheduler` (pure, testable with a fake clock) emits `liveTickMsg` through `tea.Tick`. Results arrive as `liveSnapshotMsg`, which is merged into the service cache and broadcast to the screens. New decided bouts are marked as `Fresh`.

### 10.3 Status bar states

| State | Text |
|---|---|
| Off | (nothing) |
| Waiting | `◌ LIVE · window opens 14:00 JST (in 2h13m)` |
| Polling | `● LIVE · next 2:05 · osada 16:42 JST (1m ago)` |
| Backoff | `● LIVE · ⚠ osada unreachable, using sumo-api · retry 4:00` |
| Done | `✓ LIVE · day complete · final check 18:30` |
| Not a tournament day | `LIVE unavailable · next basho Kyushu, Day 1 2026-11-08` |

### 10.4 `basho watch` (optional, phase 11)

A non-TUI live mode. It prints one line per newly decided bout and exits when the day is complete. Example:

```
16:42 JST  Makuuchi #12  M3e Oho  ○ yorikiri ●  M4w Hiradoumi
```

It accepts `--json` (newline-delimited JSON events).

---

## 11. CLI (non-interactive)

Fang wraps Cobra (styled help, `--version`, man page and shell completions).

```
basho                                   # TUI
  -t, --tournament YYYYMM  --day N  --division makuuchi|juryo  --live  --no-spoilers
basho list [--from YYYY] [--to YYYY]    # tournaments, newest first
basho info [YYYYMM]                     # metadata, yusho, sansho
basho day [N] [--tournament] [--division]    # bout list
basho bout <day> <matchNo> [--tournament]    # tale of the tape as text
basho standings [--day N] [--grid] [--tournament] [--division]
basho wrestler NAME|ID [--tournament]  # a career up to the tournament (phase 15)
basho watch [--division]                # live line stream (phase 11)
basho ratings [-t] [--top N]            # top-rated wrestlers as of a tournament (phase 5)
basho credits [--portraits]             # data sources and portrait attributions
basho version
```

Global flags:

| Flag | Meaning |
|---|---|
| `--json` | Machine-readable output |
| `--source auto\|sumoapi\|osada` | Force a data source |
| `--no-color`, `--ascii` | Plain output |
| `--no-portraits` | No wrestler portraits (also `ui.portraits = false`) |
| `--config PATH` | Alternative config file |
| `--debug` | Debug log |
| `--offline` | Snapshot and cache only, never touch the network |
| `--no-spoilers` | Spoiler-free: stop at the stored reveal horizon (also `spoilers.mode = "hide"`) |
| `--reveal-to DAY[:BOUT]` | Set the tournament's horizon (`13`, `13:19`, `j13:5` for Juryo, `0`); implies `--no-spoilers` |
| `--no-snapshot` | Hidden, for development: ignore the embedded snapshot |

Output rules:

- **When the output is a terminal (TTY):** Lip Gloss tables in the same style as the TUI.
- **When piped:** no colour, same layout. `--no-color` forces this in a terminal.
- Every data command ends with a dim provenance line (`source: snapshot (2026-10-14)`, `· stale` when an expired cache entry stood in).
- `--ascii` (or `ui.ascii`) swaps the symbols: ○ ● □ ■ – · ★ ▲ ▼ │ 結び 休 become o x O X - . * + - | musubi kyujo.
- `basho bout` prints the tale of the tape as mirrored columns, the last meetings, the result, then both technique profiles and the matchup line (§6.7). `basho standings` prints every row grouped by band (the TUI collapses the rest).
- **`--json`:** a stable envelope:

  ```json
  {
    "schemaVersion": 1,
    "kind": "day",
    "provenance": {"source":"sumo-api","fetchedAt":"…","sourceUpdatedAt":null,"stale":false},
    "data": { … domain object, camelCase … }
  }
  ```

  Enums use stable lowercase strings (`"win"`, `"fusen_loss"`, `"maegashira"`). The schema is documented in `docs/json.md`, and golden files lock it down. The schema types live in `internal/cli` and are converted from the service's views, so internal changes don't change the output. Errors go to stderr as text, also with `--json`.

- **Exit codes:**

  | Code | Meaning |
  |---|---|
  | 0 | OK |
  | 1 | Internal error |
  | 2 | Usage error |
  | 3 | Not found / not yet available |
  | 4 | Every source failed and there is no cache |

- **Tournament argument:** `-t/--tournament` and `info [ID]` accept `202609`, `2026-09` and `aki2026` / `aki-2026` (case-insensitive tournament names), and the relative forms `current` (the current or most recent tournament), `prev` and `next` (relative to `current`) (D18). Anything that can't be parsed gives exit code 2 with a list of the accepted forms.
- **Defaults:** `day` without N means today during the current tournament, otherwise the latest completed day. `--tournament` defaults to the current or most recent tournament.

---

## 12. Configuration

- File: `$XDG_CONFIG_HOME/basho/config.toml`, otherwise `~/.config/basho/config.toml`. This location was chosen deliberately on macOS (instead of `~/Library/Application Support`) so it fits a dotfiles repository.
- Precedence: flags > environment (`BASHO_*`, for example `BASHO_SOURCE=osada`) > file > defaults. Every file setting has a variable named `BASHO_<SECTION>_<KEY>` (`BASHO_LIVE_INTERVAL=5m`, lists comma separated); the session switches `--source`, `--offline` and `--debug` are `BASHO_SOURCE`, `BASHO_OFFLINE` and `BASHO_DEBUG` and never come from the file.
- Unknown settings and invalid values are errors. Intervals can only be stricter than the built-in limits (live 2 min, sumo-api 1 s, osada 20 s; §7.1). A missing file is fine unless `--config` names it.
- `basho config path` prints the file's path, without reading it, so a broken file can still be found. `basho config show` prints the configuration in effect (defaults, file, environment, flags) as TOML, marking each value that differs from the default, with the file, cache and state paths as comments. Both take `--json`.

```toml
[ui]
division      = "makuuchi"    # start division
names         = "both"        # both: romaji, kanji beside | en: no kanji | jp: kanji, romaji beside
ascii         = false
animations    = true
mouse         = true
theme         = "auto"        # auto | dark | light
win_chance    = true          # RATING and WIN CHANCE rows on the VS screen (D26)
win_chance_column = false     # day view column (toggle: p)
portraits     = true          # wrestler portraits on the VS and wrestler screens; --no-portraits for one session

[spoilers]
mode          = "show"        # show | hide

[live]
auto          = true
interval      = "3m"          # min 2m enforced
window_start  = "14:00"       # JST
window_end    = "18:45"       # JST (day 15: +30m)

[sources]
priority_history = ["sumoapi"]
priority_current = ["osada", "sumoapi"]
osada_enabled    = true
sumoapi_interval = "1s"       # time between requests; at least 1s
osada_interval   = "20s"      # at least 20s

[cache]
dir           = ""            # default XDG cache
max_size_mb   = 200

[prefetch]
adjacent_days = false
```

---

## 13. Testing strategy

- **Fixtures** (`testdata/`), recorded once with a `make fixtures` script that calls slowly, 1 request every 2 s:
  - sumo-api `202609`, finished (Makuuchi and Juryo banzuke, days 1, 13 and 15)
  - `196001`, old era: no kanji, older rank strings, possible draws
  - `202607`, day 15, to check playoffs
  - osada `torikumi.json` / `banzuke.json`, current snapshot
  - **an in-progress snapshot recorded during Kyushu 2026** (2026-11-08 to 22): several osada and sumo-api snapshots on one day, about 30 min apart, to test live merging and the "behind" detection. *Deadline-sensitive: this needs to be recorded during that tournament.*
- **Unit tests:**
  - rank parsing (English and Japanese, kanji numerals 一…十七, `横綱`, `張出`)
  - kana→romaji
  - kimarite table
  - name cleanup
  - calendar (second-Sunday estimate, cancelled tournaments)
  - every `derive` function, with as-of cutoffs and table-driven edge cases (fusen, absence, re-entry after absence, playoff, Juryo wrestler in a Makuuchi bout)
- **Snapshot tests:**
  - A three-tournament fixture snapshot (202607, 202609, 196001) is built by `basho-data` from the recorded fixtures, then loaded by the app. It lives in `testdata/snapshot/`, built by the snapshot builder in partial mode (unrecorded days are skipped); a golden test fails when it drifts from the builder or the recordings, and `go test ./internal/datatool -run FixtureSnapshot -args -update` rebuilds it. An equivalence test checks that every recorded banzuke and day reads the same through the snapshot and through the adapter.
  - The answers must equal those from the live adapter path for the same data.
  - `verify` must catch deliberately corrupted files.
- **Rating tests:** Elo update maths and the starting-rating and absence rules on a tiny hand-made bout set. A regression test pins the calibration metrics of the committed `params.json`, so a model change has to show it doesn't get worse.
- **Portrait pipeline tests:** crop, downscale and quantize are deterministic on a fixture image (golden PNG). Licence allow-list, credits generation and the half-block renderer (golden text) are covered too.
- **Adapter contract tests:** one suite runs against both adapters with their fixtures and checks that the domain output is equivalent for the same tournament and day.
- **httpx tests** (`httptest.Server`): rate limit timing (inside `testing/synctest`, so `internal/clock` stays a plain `Now()` clock), retry and jitter limits, 429 + `Retry-After`, circuit breaker state changes, ETag/304, singleflight.
- **Cache tests:** TTL per data class, stale-while-revalidate, LRU eviction, schema version change.
- **Request-budget tests:** a counting `RoundTripper` checks the numbers in §7.3.
- **Live scheduler tests:** fake clock covering the window, jitter range, doubling interval, stop conditions and day 15 extension.
- **TUI tests:** `teatest` with a fixed 120×40 and 80×24 size and a fake clock. Golden snapshots of each screen; key flows (picker → day → VS → reveal → back); resizing; spoiler-free flow. Screens are mostly tested through `tuitest.Driver`, which runs the model synchronously with timers off (`tui.NoTimers`); goldens are plain text.
- **CLI tests:** golden text output and JSON schema golden files (`internal/cli/testdata/TestGolden/`, refreshed with `go test ./internal/cli -run Golden -update`). They run over the fixture snapshot, which does not change with `make snapshot`; an acceptance test checks the Phase 6 commands' facts against the embedded snapshot without remote layers.
- **Live smoke tests** behind `//go:build live`, never part of `make check`: one request per endpoint to catch API changes.
- Tooling: `golangci-lint`, `gofumpt`, `go test -race`, all through `make check` (D28).

---

## 14. Build, release, distribution

- **Go version:** the current stable release (1.27 locally); the `go` directive in `go.mod` follows it (currently `go 1.27`).
- **JST** is a fixed UTC+9 zone (`internal/domain`), exact for 1960 onwards, so no time zone database is needed at runtime.
- **Task runner:** a `Makefile` (§14.1). `make check` is the full check, run locally before committing (D28).
- **Build information:** only the version (`git describe --tags --always`) is set via `-ldflags -X` on `internal/buildinfo`. Commit, dirty flag and commit time come from the Go toolchain's VCS stamping, so there is one source for them. The snapshot date isn't an ldflag either: `basho version` reads it from the embedded manifest, so it can never be out of sync.
- **Build targets:** darwin/arm64, darwin/amd64, linux/amd64, linux/arm64. Only `cmd/basho` is released; `cmd/basho-data` is never shipped.

### 14.1 Makefile

`make` with no target runs `help`. Variables: `GO ?= go`, `BIN := bin`, `ARGS` (passed to `run` and the data tool), `LDFLAGS` with the version from `git describe --tags --always`.

| Target | What it does | From phase |
|---|---|---|
| `help` | Lists all targets with a one-line description (parsed from `##` comments) | 0 |
| `build` | `go build` both commands with ldflags into `bin/basho` and `bin/basho-data` | 0 |
| `install` | `go install ./cmd/basho` with ldflags | 0 |
| `run` | `go run ./cmd/basho $(ARGS)` | 0 |
| `test` | `go test ./...` | 0 |
| `test-race` | `go test -race ./...` | 0 |
| `test-leaks` | The screen walks (spoiler leaks, `--ascii`, `ui.names`) without the race detector, under which they skip (single-threaded, ten times slower there) | 11 |
| `test-live` | `go test -tags live ./...`: real API smoke tests, manual only | 1 |
| `golden` | Re-runs tests with `-update` to rewrite golden files (review the diff afterwards) | 6 |
| `bench` | The performance budgets: first screen under 300 ms, grid frame under 16 ms (§17 Phase 12) | 12 |
| `cover` | Coverage profile and HTML report (`coverage.html`) | 0 |
| `fmt` | `go tool gofumpt -w .` | 0 |
| `fmt-check` | Fails if gofumpt would change anything | 0 |
| `lint` | `golangci-lint run` | 0 |
| `vuln` | `go tool govulncheck ./...` | 0 |
| `tidy` | `go mod tidy` and `go mod verify` | 0 |
| `check` | `fmt-check lint vuln test-race test-leaks`; the full check before committing | 0 |
| `tools` | Checks that external tools exist (golangci-lint; goreleaser from Phase 17) and prints install hints | 0 |
| `fixtures` | `basho-data fixtures record`: polite recording of the test fixtures | 1 |
| `record` | `basho-data record $(ARGS)`: live recorder (Appendix E) | 1 |
| `snapshot` | `basho-data snapshot update` then `verify` | 3 |
| `snapshot-verify` | `basho-data snapshot verify` | 3 |
| `snapshot-stats` | `basho-data snapshot stats` | 3 |
| `calibrate` | `basho-data rating calibrate`; writes `data/rating/params.json` and `docs/rating.md` | 5 |
| `vhs` | Renders the README GIF from `docs/demo.tape` (needs VHS: `brew install vhs`) | 12 |
| `release-snapshot` | `goreleaser release --snapshot --clean` into `dist/` | 17 |
| `portraits` | `basho-data portraits update` | 14 |
| `portraits-preview` | Rebuilds only the wrestlers in `overrides.toml` into a preview sheet, from the work cache | 14 |
| `clean` | Removes `bin/`, `dist/` and coverage files; never the work cache or `data/` | 0 |

- **Placeholders:** every target exists from Phase 0. Targets whose phase hasn't arrived yet print "not implemented yet (phase N)", so `make help` shows the whole picture from the start.
- **Network targets** (`fixtures`, `record`, `snapshot`, `portraits`) print the planned number of requests and ask for confirmation unless `YES=1` is set.

### 14.2 v1.0 (private repository)

- `make install`: `go install` of `./cmd/basho` with the ldflags. This is the main way to install v1.0.
- **GoReleaser v2** for reproducible builds:
  - archives for the targets above, checksums, completions and man page (from Fang)
  - `make release-snapshot` (`goreleaser release --snapshot --clean`) for local test builds
- **Release flow after a snapshot update:** commit the snapshot, tag the next patch version (`v1.2.3` → `v1.2.4`), run `make install`.
- **Brewfile in `macos-setup`:** nothing for v1.0. Optionally a `config.toml` in its `home/` directory.

### 14.3 After 1.0: public repository and Homebrew tap

Only once you have decided to make the repository public (D9, D22).

- GoReleaser's `homebrew_casks` section has `repository: {owner: thorstenpfister, name: basho, branch: main}` and `directory: Casks`. On every release it commits `Casks/basho.rb` to `main`.
- Releases run locally: `GITHUB_TOKEN=$(gh auth token) goreleaser release --clean` after tagging. It publishes the GitHub release and commits the cask (D28).
- Installing needs an explicit URL, because the repository isn't named `homebrew-*`:
  ```
  brew tap thorstenpfister/basho https://github.com/thorstenpfister/basho
  brew install --cask thorstenpfister/basho/basho
  ```
- Brewfile in `macos-setup`:
  ```ruby
  tap "thorstenpfister/basho", "https://github.com/thorstenpfister/basho"
  cask "thorstenpfister/basho/basho"
  ```
- **The repository must be public**: Homebrew downloads release assets without credentials.
- **Unsigned (D25):** the cask gets a `postflight` hook that removes the quarantine attribute (the approach GoReleaser documents for unsigned casks).
- The tap clones the whole repository, including the data snapshot (a few MB, growing slowly), which is acceptable.
- Linux: release tarballs and `go install github.com/thorstenpfister/basho/cmd/basho@latest`.

### 14.4 README

- A VHS tape (`charmbracelet/vhs`) generating a GIF of the main flow, install instructions, data source credits (sumo-api.com, osada.us with its AI-use notice, Wikimedia contributors for portraits).

---

## 15. Future ideas (not in v1)

- Favourites, filters, notifications (decided against for v1).
- `basho cache info|clear` (not selected for v1; `--offline` and the automatic cache cover the basics).
- "Streak" markers (🔥 5 wins in a row).
- Theme packs (for example "arcade" with a block-font `VS` and a matching portrait palette).
- Signing and notarization (D25).
- A better rating model (Glicko-2, or a blend with rank and head-to-head) if plain Elo shows weaknesses. The overconfidence in V9 comes from 2015–2020, not from Elo in general (Phase 13 step 1). A likely cause is top wrestlers fighting hurt or withdrawing partway through a tournament: their ratings stay high, and the model only pulls ratings back after tournaments missed completely. Modelling partial absences would be the place to start.
- Full-resolution portraits via the Kitty graphics protocol in terminals that support it, if Bubble Tea gains a clean way to mix it with text rendering.

---

## 16. Open questions and things to check

**Decided:**

- ~~Q1 Binary name~~ → `basho` (D11)
- ~~Q2 VS screen extras~~ → FROM and AGE kept, technique profile added (D12)
- ~~Q3 Live window and division~~ → both divisions, from 14:00 JST (D13)
- ~~Q4 Key conflicts~~ → `v` for head-to-head (D16)
- ~~Q5 Default spoiler horizon~~ → Day 0 (D14)
- ~~Q6 Language of labels~~ → English only (D15)
- ~~Q7 Technique profile~~ → five categories as drafted (D17)
- ~~Q8 Tournament flag format~~ → all friendly forms plus `current`/`prev`/`next` (D18)

**Open:**

- ~~Q9 sumo-api.com permission~~ → no contact; within fair use (D24)
- ~~Q10 Public repository~~ → decide after 1.0; tap moves after 1.0 (D9, D22)
- ~~Q11 Notarization~~ → not for now (D25)
- ~~Q12 Portraits~~ → before 1.0 (D23)
- ~~Q13 Win chance visibility~~ → shown by default on the VS screen (D26)

**To check (V = verification):**

- **V1** How quickly sumo-api results appear during the live window. Measure during Kyushu 2026 (2026-11-08 to 22) by recording `resultUpdatedAt` against the first appearance of each result in sumo-api.
- ~~**V2**~~ **Answered (Phase 1):** sumo-api's bout lists contain **no playoff bouts**: on day 15 of 202609 nobody fights twice in Juryo although osada shows a two-bout Juryo playoff; only the yusho winner records the result. A day can still have an odd number of entries when a Juryo wrestler fills in (202607 day 15: 20 Makuuchi bouts). Playoffs therefore exist only for the current tournament, via osada.
- ~~**V3**~~ **Answered (Phase 1):** the fixtures (196001, 202607, 202609) contain only `win`, `loss`, `fusen win`, `fusen loss` and `absent` (kimarite `fusen` or empty); W-L-A computed from the records matches sumo-api's own totals for every wrestler. Draws exist as kimarite values (`hikiwake`, `itamiwake`, V11) but did not occur in these tournaments; an unrecognised result maps to `Unknown`, so the full snapshot build will surface any.
- ~~**V4**~~ **Answered (Phase 1):** `limit=1000` returns 1000 records; the index has 9,132 wrestlers, so 10 pages. Larger limits were not tried.
- ~~**V5**~~ **Answered (Phase 1):** sumo-api sends no `ETag` or `Last-Modified`; its responses can only be cached by time (§7.2). osada sends ETags, weak ones (`W/"…"`) when gzip is negotiated, and answers both forms with 304.
- ~~**V6**~~ **Answered (Phase 1):** decided playoff bouts appear in `resultDays` with `isPlayoff: true`, a kimarite and a winner (202609 Juryo: bouts 15 and 16). The `未定` seen earlier was in `scheduleDays`.
- ~~**V7**~~ **Answered (Phase 1):** in the banzuke fixtures every entry's rank string matches the list it is on. The "Ozeki 2 East" seen earlier came from a match record, not the banzuke; the rank string is authoritative anyway.
- **V8** Actual snapshot size (compressed) and time to build the bout index at startup. Answered: 2.50 MB for 399 tournaments (§4.3); opening the snapshot and building the whole bout index takes about 50 ms on an Apple M5.
- ~~**V10**~~ **Answered (Phase 1):** `rankValue` = title × 100 + number (Yokozuna 1 → 101, Juryo 3 → 603); the side is not encoded, East and West share the value. Old-era haridashi wrestlers appear as ordinary numbered ranks (196001 has "Yokozuna 2 East"), so the English format is the same throughout. Side note for §6.8: Juryo had 18 ranks per side in 1960, not 14, so the rank bands must not assume J14 is the lowest.
- ~~**V11**~~ **Answered (Phase 1):** sumo-api.com's `/kimarite` lists 90 values, all spelled as in Appendix B. Four were missing and are added: `dashinage` and `yobikaeshi` (historical), `hikiwake` and `itamiwake` (old-era draws). `kakezori`, `shumokuzori` and `sototasukizori` are official but never occur in sumo-api's data.
- **V9** Whether Elo beats the "higher rank wins" baseline on log loss for 2015 onwards. This decides whether the win chance is shown at all. **Answered: yes, shown.** On 34,759 bouts from 2015 to 202609, Elo (`K` 10, `K_new` 24 for the first 45 bouts, `r` 0.3, warm-up scale 0.5) has log loss 0.6772, Brier 0.2422 and 56.2% accuracy, against 0.6860 / 0.2465 / 54.0% for the rank baseline and 0.6931 / 0.2500 / 50% for a coin. The margin is modest, as expected for a sport where most bouts are close. The model is somewhat overconfident at the extremes (predicted 74% wins 69%, 93% wins 85%; lowering `K` does not fix it). Phase 13 found this belongs to one era rather than to Elo itself. The win chance's best divisor, fitted era by era, is 380–434 from 1965 to 2014 and 428 for 2021–2026, all close to 400, but 562 for 2015–2020. Fitted on the training years, it is 406, which moves evaluation log loss only from 0.6772 to 0.6770. So the chance keeps 400; see §15. Report: `docs/rating.md`.

---

## 17. Implementation plan

Each phase ends with something usable and has acceptance criteria (AC). Phases 1–6 build and prove the data foundation through the CLI before the TUI starts.

**Working agreement** (how the phases are implemented):

- One sub-step at a time: implement → tests green → review for reuse, simplification, efficiency and altitude (one combined review) → tests still green → a full code review for risky logic (retries, concurrency, state machines) → `make check` → **one commit per sub-step** → a short summary and a preview of the next sub-step. A whole phase may be run in one go when the owner asks, with an overview at the end.
- Review findings are applied or explicitly skipped with a reason; format and design changes found this way are written back into this spec in the same commit.
- Network-heavy targets (`make snapshot`, long `record` runs) are run by the owner; small smoke tests (fixtures, a single tournament) may be run during development.
- No CI workflows (D28); `make check` before every commit. The `go` directive tracks the locally installed Go release.

**Deadline-sensitive, outside the phase order:** during Kyushu 2026 (2026-11-08 to 22), record the in-progress fixtures and measure V1 with the live recorder (Appendix E). It must be ready by **2026-11-07**, so it is built as Phase 1 step 9, ahead of everything else in that phase if needed.

### Phase 0: Project scaffolding

1. `go mod init github.com/thorstenpfister/basho`; create the directory layout from §5.1 with placeholder packages.
2. Add the dependencies from §5.9, including the `tool` directives for gofumpt and govulncheck. Before writing TUI code, read the actual Charm v2 sources and examples, because the v2 APIs differ from v1.
3. `cmd/basho/main.go` with Fang and `basho version` (build information via ldflags); an empty `cmd/basho-data/main.go`.
4. Tooling: `Makefile` with every target from §14.1 (later targets print "not implemented yet" until their phase), `.golangci.yml` (including a depguard rule that bans the v1 Charm imports `github.com/charmbracelet/bubbletea`, `lipgloss`, `bubbles` and the non-`/v2` `x/exp/teatest`, in test files too), `tool` directives in `go.mod` (§5.9), `.editorconfig`, `.gitignore` (`bin/`, `dist/`, `.cache/`, `coverage.*`).
5. `clock` package (real and fake clock); JST and day helpers in `domain/jst.go`.
6. Short README with a project description and data source credits.

**AC:** `make check` and `make build` pass; `make help` lists every target; `basho version` prints version, commit and date.

### Phase 1: Domain model and sumo-api adapter

1. `domain`:
   - the types from §5.2
   - `Rank` parsing for English strings, `Short()`, `SortKey()`
   - the `Outcome` enum and `BashoID` validation
   - the calendar: fixed months, second-Sunday estimate, list of cancelled tournaments, venue by month, tournament names
   - the date representation: decided as `domain.Date`, a value type (year, month, day) that is `==`-comparable, works as a map key and prints as `2026-11-08` (§5.2)
   - tournament argument parsing (D18)
2. Kimarite table as embedded data: 82 techniques plus non-techniques, each with romaji, kanji, English gloss and moveset category (§6.7).
3. Name cleanup (§5.6).
4. `source/sumoapi` DTOs for basho, banzuke, torikumi and rikishis (paged). The "zero object" for unknown tournaments becomes `ErrNotYetAvailable`; data that cannot be mapped is `ErrBadData` (not retryable, unlike `ErrUpstream`). `rikishi`, `matches` and `stats` are deferred: they are only a fallback without a snapshot (§5.8). The §5.3 interfaces are declared with the resolver (Phase 9).
5. Mapping from DTOs to the domain, with an explicit strategy for unknown values.
6. A slow, polite fixture recorder (`make fixtures`) and the fixtures from §13.
7. Unit tests: rank parsing, calendar, tournament argument parsing, mapping for 202609, 196001 and 202607.
8. Answer V2–V7, V10 and V11 from the fixtures and record the answers in this spec. Done; also found that sumo-api records height and weight only at the debut (§4.1, §6.6).
9. Live recorder `basho-data record` (Appendix E), on a minimal `httpx` (User-Agent, timeouts, conditional requests, minimum interval per host) that Phase 2 extends. Runs on an always-on machine via launchd; see `docs/runbooks/kyushu-recorder.md`.

**AC:** a test can load the 202609 Makuuchi banzuke and the day 13 bout list from fixtures into domain objects, with every outcome type represented; V2–V5 and V7 are documented.

### Phase 2: Polite HTTP and cache

1. `httpx`: client with User-Agent, gzip and timeouts; per-host rate limit; concurrency limit; retries with jitter; 429 and `Retry-After`; circuit breaker; ETag/Last-Modified; singleflight; debug log.
2. `cache`:
   - file store with entry metadata and the TTL table from §7.2
   - stale-while-revalidate: the caching `Getter` revalidates expired entries with conditional requests and falls back to the stale copy when the source fails; showing stale data first and refreshing in the background is the service layer's part (Phase 6)
   - LRU size cap, schema versioning, `--offline` support
3. Data-class classification, including detecting finished tournaments (end date + 2 days and a yusho winner present). Each adapter supplies a freshness policy that sees the URL, status and body (the yusho is in the body), and the caching `Getter` applies it at read time. Banzuke responses carry no dates or yusho, so they go by the estimated end date.
4. Connect the sumo-api adapter to `httpx` and `cache`.
5. Counting RoundTripper and request-budget tests (§7.3, sumo-api rows).
6. httpx and cache test suites (§13).

**AC:** opening a finished tournament twice makes exactly one set of requests; simulated 429s and 5xx cause the expected backoff and circuit behaviour; a second run with `--offline` works for data already viewed.

### Phase 3: Snapshot tool and embedded data

1. Snapshot file formats (§4.3): manifest, per-tournament file, wrestler index; zstd encoding and decoding; schema version.
2. `datatool` snapshot builder:
   - incremental, based on the manifest
   - `--from`/`--to`
   - `--dry-run` printing the request plan and an estimated duration
   - resumes through the work cache
   - uses the `httpx` limits, which can only be made stricter
3. `basho-data snapshot verify` and `snapshot stats` (sizes, coverage, bouts per tournament).
4. `internal/snapshot` loader with `go:embed`, and the layer order snapshot → cache → network in the service plumbing (§5.8).
5. Bout index (by wrestler and by tournament), built lazily and extendable with later tournaments.
6. Fixture snapshot (3 tournaments) for tests; equivalence tests between the snapshot path and the adapter path.
7. Run the full build once (about 4 hours, not on a tournament day), measure V8, commit `data/snapshot/`. Done 2026-10-13/14; the build found the source gaps now handled in §4.3.

**AC:** with network access blocked, any tournament from 196001 to the snapshot's newest one loads in under 100 ms; `verify` passes on the committed snapshot; a second `snapshot update` without new tournaments makes no requests, apart from one metadata request for a tournament past its estimated end that is not final yet (cached for an hour).

### Phase 4: Derived data engine

1. The `AsOf` cutoff type and helpers.
2. Record going into a bout and after it, including absences and fusen.
3. Last 5 across tournament boundaries from the bout index, padded for newly promoted wrestlers.
4. Kinboshi count and `ThisBashoScalps`.
5. Head-to-head summary (wins, top techniques, last N meetings).
6. Standings after day N: sorting, bands, contention calculation, yusho decided, kachi-koshi/make-koshi markers including "reached today", hoshitori grid model.
7. Career statistics as of a date: height and weight from the index ("latest" for older tournaments), age, exact yusho/sansho as of the tournament, "now: X" when the shikona has changed.
8. Day recap: leaders, kachi-koshi/make-koshi reached today, kinboshi today, upsets (using the rating once Phase 5 is done; rank gap until then), rarest technique.
9. Technique profile (§6.7): category shares, sample window with widening, signature move, fighter type, matchup line. Tune the fighter-type thresholds against about 10 well-known wrestlers and record the final values in this spec.
10. Table-driven tests for each, including edge cases: absence then return, fusen against a yokozuna, playoff, Juryo wrestler in Makuuchi, fewer than 10 sekitori wins.

Done 2026-10-14. The acceptance tests run on the embedded snapshot: 202609 standings against sumo-api's own totals, the kinboshi of Akinoshima (16), Takamiyama, Tochinonada (12) and Tosanoumi (11) against Wikipedia, and the fighter types the thresholds were tuned on; bout-index build time is in §16 (V8).

**AC:** standings for 202609 after day 15 match the official final records; kinboshi counts for 3 chosen wrestlers match published totals (checked by hand once against Wikipedia); bout-index build time is recorded (V8).

### Phase 5: Rating and win chance

1. `rating` package: Elo update, rank-band starting ratings with the fixed table for the warm-up period, `K_new`/`K`, adjustment after absences, as-of queries, continuing through live results.
2. `basho-data rating calibrate`: grid search, train/test split, log loss, Brier score, accuracy, calibration table, both baselines; writes `data/rating/params.json`.
3. **Decide V9** and record the metrics in this spec. If Elo doesn't beat the rank baseline, the win-chance UI stays switched off and a revised model goes on the list. Done: shown (§16, V9).
4. Plausibility check: the top-rated wrestlers of all time compared with published Elo lists (Ozeki Analytics) and common sense. Done: the calibration report lists the 10 highest peaks (`docs/rating.md`). With the chosen parameters they are Taiho (2072, 196711), Hakuho (2054, 201101), Kitanoumi, Tamanoumi, Asashoryu, Chiyonofuji, Kitanofuji, Kashiwado, Takanohana and Wajima: all dominant yokozuna. Ozeki Analytics (all divisions from 1958, K 20) has Hakuho first, peaking in 2021, and notes rating inflation among top yokozuna; with sekitori bouts only and a lower K, our Hakuho peaks during his 2010 winning streak, just behind Taiho.
5. Service output: rating, change since the tournament start, win chance; `basho ratings` command. Done: `service.Ratings` (as-of rating with change since the tournament start, win chance, upsets, live `Set`) on a lazy `service.Service`; `basho ratings` prints plain text for snapshot tournaments, and the remote layers, `--json` and exit codes follow in Phase 6.
6. Tests from §13 (maths, rules, metric regression). Done: the regression test reruns the committed `params.json` over the embedded snapshot and must reproduce its evaluation scores and rank baseline exactly, which also proves the model deterministic.

**AC:** the calibration report is committed under `docs/rating.md`; `params.json` is embedded; win chances for a sample of bouts are plausible and identical between two runs (deterministic).

### Phase 6: Service layer and non-interactive CLI

1. `service` façade: `CurrentBasho`, `Basho`, `Day(basho, division, day)`, `Bout(basho, division, day, matchNo)` (the full tale-of-the-tape model, including moveset and rating), `Standings(basho, division, day)`, `Recap`, `Ratings`. Each returns `Result[T]` with provenance. Done as `CurrentBasho`, `Info`, `List`, `DefaultDay`, `Day`, `Bout`, `Standings`, `Recap` and `RatingTable`. `Info` names Makuuchi and Juryo winners always and lower-division winners only when the wrestler index knows them (it holds sekitori).
2. Background refresh of the active-wrestler index when the snapshot's index is older than 7 days, plus the `idmap` skeleton (filled in Phase 9). Done: `service.RefreshWrestlers` fetches `/rikishis` without `intai` (the ~600 active wrestlers, one page) once the snapshot is more than 7 days old and overlays their current profiles; `StartBackground` runs it for long sessions (the TUI), while one-shot CLI commands don't wait for it. `idmap.Map` maps JSA IDs from the index.
3. Config loading (file, environment, flags, defaults) and validation, including minimum intervals and maximum rates. Done (`internal/config`, BurntSushi/toml); the per-host intervals are the `sources.*_interval` settings (§12).
4. Commands: `list`, `info`, `day`, `bout`, `standings` (`--grid`), `ratings`, `credits`, with text renderers (Lip Gloss tables in a terminal, plain when piped). Done; `service.Open` wires the production layers from the configuration (snapshot, sumo-api over the disk cache and `httpx`, debug log). `--source osada` is refused until Phase 9.
5. The `--json` envelope, `docs/json.md` and exit codes. Done; `service.NotFound` and `service.Unavailable` classify failures for exit codes 3 and 4.
6. Golden tests for text and JSON. Done.

**AC:** `basho day 13 -t 202609`, `basho bout 13 19 -t aki2026` and `basho standings --day 15 -t 2026-09 --grid` give correct, readable output with no network access (snapshot), and their `--json` versions match the golden files.

### Phase 7: TUI shell, picker and day view

1. Theme package: adaptive palette, styles, Unicode and ASCII glyph sets, `NO_COLOR`. Done (`tui/theme`); `NO_COLOR` and `--no-color` act where Bubble Tea writes, through the colour profile.
2. Root model: screen stack, global keys (§8.8), size breakpoints, "too small" screen, status bar with provenance and short-lived messages, help overlay. Done (`tui.Model`). Screens are values behind a `Screen` interface sharing a `Context` (service, config, theme, clock); the app starts with the picker under the day view, so `esc` pops back to it. `L` and `S` show a message until Phases 10 and 11.
3. Basho picker built from the calendar; yusho text from the snapshot (instant for history) and the cache; `/` filter; cancelled rows. Done: rows from the upcoming tournament back to 196001 with year separators; winners load once in the background through `service.CachedOnly` (snapshot and disk cache, never the network); `/` matches years, names and month numbers, every word must match, and the root lets a focused input capture the keys (`tui.KeyCapturer`).
4. Day view:
   - day bar with status marks
   - bout table (winner bold, loser dim, records, techniques, `結び`, playoff divider)
   - Kyujo footer
   - `d` division toggle, number and `g` jumps
   - `p` win-chance column

   Done: `basho` starts it on the current tournament's latest day with the picker underneath. The day bar comes from `service.DayStatuses` (snapshot tournaments count their bouts, so gap days such as 196503 day 7 can't be selected; later tournaments go by the dates, one metadata request at most). The compact layout caps names and techniques at 10 cells and drops the bout number. `enter`, `s` and `r` show a message until Phases 8 and 9.
5. Loading, empty and error states; stale-while-revalidate redraws; `R` refresh with cooldown. Done: a day loads through `service.CachedOnly` first (snapshot or cached copy, even expired), then, unless it came from the snapshot, through the sources in the background with `↻`; a failed refresh keeps the copy and says so. `R` revalidates with a conditional request (`service.Revalidate`; pinned, final entries are never refetched), at most every 30 s per day. Timers go through `tui.Context.Tick`, so the spinner and messages don't run in tests.
6. Wide layout side panel (top bands of the yusho race). Done: from 130 columns, the leaders and the −1 band after the selected day, loaded cache-only once the day is in.
7. `teatest` golden snapshots (80×24, 120×40) and navigation flow tests. Done (`internal/tui/screens_test.go`): plain-text goldens of the start screen, Juryo, day 13, the picker with and without a filter and the help at both sizes, over the fixture snapshot; flow tests through a real program with `teatest` (the renderer writes only changed cells after the first frame, so the flows check the final model); every day since 1960 in both divisions from the embedded snapshot without remote layers; and a resize sweep from 50×15 to 200×50 in which every screen fills the terminal exactly.

**AC:** starting `basho` lands on the latest tournament day; every day since 1960 can be browsed in both divisions without network access, except tournaments newer than the snapshot; resizing never breaks the layout.

### Phase 8: VS screen

1. Components:
   - stat row (mirrored values around a centre label)
   - bar, scaled to the division's range for that tournament
   - result glyphs, kinboshi stars
   - win-chance bar

   Done (`tui/components`): bars map the division's lightest or shortest to 20% of the cells and the heaviest or tallest to all of them; the STYLE bar splits its 20 cells by largest remainders; ASCII mode draws `#` bars and P/B/T/L/O letters.
2. Layout of the tale of the tape (§8.5), compact and wide variants, head-to-head line and expandable history (`v`). Done (`tui/bout`): the tape is at most 110 columns; compact drops the bars; wide adds a 42-column head-to-head column with up to 10 meetings, which `v` shows below the tape otherwise. The service now carries all three moveset windows and up to 10 meetings (`service.MaxMeetings`); the CLI and JSON keep showing the last-6-basho window and 3 meetings.
3. Progressive loading for tournaments not in the snapshot: draw immediately from bout and banzuke data, fill in the rest with `…` placeholders. Done: the VS screen draws names, kanji, ranks and records from the day view's row at once and `…` for every other row until the tale is in (cache first, then the sources, like the day view).
4. Result panel: winner, technique (romaji, kanji, English), records after the bout, KINBOSHI and FUSEN stamps, undecided and waiting states. Done: an undecided bout reads "Waiting for result…" on its own day and "Scheduled — Day N" otherwise; while the tale loads, the panel uses the day view's row.
5. Technique rows: fighter type under the name, mirrored STYLE stacked bar, SIGNATURE row, "not enough data" state. `m` opens the moveset overlay, `w` cycles its window, and it shows the matchup line. Done; the compact layout keeps the STYLE bar at 10 cells (its "without bars" means height and weight). The overlay replaces the tape and keeps the result panel; `w` acts only while it is open.
6. RATING and WIN CHANCE rows, hidden if V9 failed. Done; also hidden by `ui.win_chance = false` (D26). A wrestler without a rated bout yet is marked `new`.
7. `[`/`]` navigation including crossing day boundaries. Done: days without bouts are skipped, and the first and last bout of the tournament say so. The day view underneath stays where it was.
8. Harmonica slide-in and reveal flash; switched off by config, compact layout, or when the output isn't a terminal. Done: a critically damped spring (ω 26 at 60 fps) settles in under 20 frames; frames run on `Context.After`, and `tui.Context.Animate` combines `ui.animations`, the compact layout and `Context.TTY` (stdout is a terminal). The flash answers `bout.RevealMsg`, which spoiler-free mode sends (Phase 11).
9. Golden snapshots for: a regular bout, a kinboshi bout, a fusen bout, an undecided bout, an old-era bout without kanji, a newly promoted wrestler. Done over the fixture snapshot (plus the compact and wide variants of the regular bout; the undecided bout comes from a fake remote); the acceptance tests render every Makuuchi bout of 202609 at 90 and 130 columns from the embedded snapshot and prove with a failing remote that a snapshot bout, stepped through with `[`/`]`, makes no requests. Height and weight carry a dim `*` outside the current tournament, explained in the help legend.

**AC:** every Makuuchi bout of 202609 renders without layout problems at 90 and 130 columns; opening a VS screen for a snapshot tournament makes no requests.

### Phase 9: Standings, recap and osada adapter

1. Standings screen:
   - tab A: race, bands, collapsed tail, contention and yusho markers
   - tab B: hoshitori grid with the day column highlighted
   - `←`/`→` move the cutoff; wrestler popup

   Done: `s` in the day view opens `tui/standings` after the shown day. The race has one line per score (wins, then losses and absences together, as it is sorted), wrapped when it holds more wrestlers than fit; wrestlers out of contention are dim, the leaders bold, and the tail below −2 is one line until `enter` expands it. The grid's cursor (`j`/`k`) opens the popup with each day's opponent (visitors from Makushita named from the tournament's bouts) and technique. `d` switches division here too, and the footer names the yusho, a playoff or "Yusho decided: no".
2. Day recap overlay (`r`). Done: a box over the day view under the day bar, loaded from the cache (the day is cached by then). It takes every key while open: `r`, `esc`, `q`, `b` and `enter` close it, and `←`/`→` move the day with the recap following. Early days name six leaders and "and N more". The technique of the day says how often it decided a bout in earlier tournaments.
3. `source/osada`:
   - DTOs for `torikumi.json` and `banzuke.json`
   - Japanese rank parser (kanji numerals, `枚目`, `張出`)
   - Japanese technique names → canonical keys (via the kimarite table); `未定` → pending
   - `isPlayoff`, absentees, status and timestamps
   - JSA ID taken from the profile URL

   Done: `osada.Client.Current` reads `banzuke.json` and `torikumi.json` into `source.Current` (JSA IDs, both divisions, published days with the results where they are out and the schedule otherwise). Findings from the recorded Aki 2026 files:
   - The banzuke writes absences as `draw`: Hoshoryu, kyujo all tournament, has 15. They map to absent; the modern era has no draws.
   - Bout ranks carry no side, and san'yaku titles no number (`大関` for O2e). Ranks therefore come from the banzuke, which lists each rank level's east and west wrestlers in order. Makushita visitors keep the bout list's rank without a side.
   - The JSA spells 浴せ倒し where the kimarite table has 浴びせ倒し; the adapter keeps such spellings. Unknown names pass through.
   - The tournament comes from the banzuke's `令和八年` + `九月場所`. When the bout list's `pathDate` names another tournament (the next banzuke is out), the days are left out.
4. `idmap`: JSA ID ↔ sumo-api ID from the snapshot index plus the active index; refresh on an unknown ID (at most once an hour); kana→romaji fallback marked with `≈`.

   Done: `idmap.Map.Resolve` maps a JSA ID to the sumo-api wrestler with the index's romaji, and osada's kanji and kana. An unknown ID refetches the active wrestlers once an hour at most (revalidated past the 7-day cache, failures count too) and overlays them on the profiles. A wrestler still unknown gets a temporary identity: ID minus the JSA ID, and a name transliterated by `idmap.Romaji` with `Name.Approx` set. Screens and the CLI print names with `Name.Romaji()`, which puts `≈` in front; JSON carries `"approx": true`. Collapsing `ou` also hits morpheme boundaries (しょうなんのうみ → Shonannomi), which the `≈` admits.
5. `resolver`: the policy table (§5.4), "behind" detection, merge rules, per-bout source tracking, `--source` override, stale cache as last resort.

   Done: `resolver.Resolver` answers the service's Basho/Banzuke/Torikumi in sumo-api's place; `service.Open` puts it over both adapters, each over its own cache policy (osada: the live interval in the window, 30 min outside, never pinned).
   - **Current** means from 14 days before the estimated day 1 (the banzuke is out) until the results are final; only then is osada asked, and only when it serves that tournament.
   - **Fallback** (rule 1) on upstream errors, 429, a paused host, nothing cached in cache-only mode, and also on not-found / not-yet-available, which is how a source that is behind shows. Unmappable data does not fall back.
   - **Behind without a request:** the bout list from osada is compared with sumo-api's copy only when that is in the cache (cache-only), so live polling stays at one osada request (§7.3).
   - **Stale** (rule 4): a source that answers only from an expired cache copy gives way to the next one; when that fails too, the stale answer is served.
   - **Provenance:** a `resolver.Track` on the context records which sources answered and their responses; the status bar names them all ("osada + sumo-api"), marks `(forced)` under `--source`, and shows osada's `resultUpdatedAt`. `BoutRow.Source` names a bout's source when a day merged both.
6. Contract tests: both adapters produce equivalent domain output for the current-tournament fixtures.

   Done: over the recorded Aki 2026 files, osada mapped through the JSA IDs gives sumo-api's banzuke exactly (ranks, romaji and kanji, all 15 results with opponents, techniques, fusen and absences) and the same bouts on days 1, 13 and 15. Differences by design: osada adds kana and the playoffs, and gives Makushita visitors no side. The AC run the day view and standings on day 13 without the snapshot: sumo-api down (osada alone, English names through the mapping, none transliterated), osada down (sumo-api alone), and both up, where the status bar reads "sumo-api + osada".

**AC:**
- With sumo-api disabled (in a test), the current tournament's day view and standings still render from osada, with English names via the ID mapping.
- With osada disabled, everything falls back to sumo-api.
- The status bar always names the source that answered.

### Phase 10: Live mode

1. `live.Scheduler`, pure and testable with a fake clock: windows from config, day 15 extension, jitter, doubling when nothing changes, stop conditions, final check, error backoff.

   Done: every method takes the time. `State` (off, waiting, polling, backoff, done, unavailable), `Due` and `Wake` (the next poll, the window opening, the final check, or tomorrow's window) drive the TUI's timer; `Polled` takes a poll's update stamp, completeness and error. Doubling counts two polls in a row with the same stamp; backoff runs 2, 4, 8, 10 minutes; a new date starts the day afresh. The final check runs even after the window has closed, as one more poll.
2. Wire it into the TUI:
   - `liveTickMsg` / `liveSnapshotMsg`, merged into the service cache, bout index and ratings
   - `NEW` badges, selection kept stable
   - status bar states (§10.3), `L` toggle, `live.auto`

   Done: the root owns the scheduler for the tournament in progress (`service.LiveTournament`) and runs `service.LivePoll`, both divisions of today. Its answer, a `tui.LiveMsg`, goes to every screen on the stack:
   - The day view showing today takes the poll's data as it is: newly decided bouts get a `NEW` badge (until the next poll, 60 s at most), and the cursor stays put. Its day bar, race panel and recap reload from the cache.
   - The standings reload from the cache when the poll's day is within their cutoff.
   - `L` toggles live mode, and turning it off keeps `live.auto` from turning it back on that day. While live mode polls, `R` polls at once (30 s cooldown) instead of refreshing the screen.
   - The status bar's right part shows the live state in place of the screen's provenance; "day complete" still names the source.
   - **Budget detail:** a poll marks its context with `resolver.Live`. osada's `torikumi.json` is revalidated once per tick (sumo-api's bout lists instead, when osada fails); osada's `banzuke.json` keeps a 30-minute TTL and is never revalidated; sumo-api's metadata and banzuke come from the cache, even expired, with osada's results laid over them. A cache miss in a cache-only read is not a source failure.
   - **Completeness:** day 15 is only complete when no playoff is still to come: a shared lead nobody can change without listed playoff bouts keeps it open.
3. Conditional GET for osada (the 304 path), with the sumo-api fallback each tick.

   Done: tested end to end over HTTP servers with ETags through the production cache stack. After the first poll, each poll is exactly one request, a 304 for `torikumi.json`, and no sumo-api requests. The first poll fetches whatever was never cached (osada's two files, sumo-api's metadata and banzuke) and then does not revalidate again. Each live poll revalidates once per source: the bout list's osada read, and sumo-api's bout lists only when osada fails. A source that can answer only from an expired copy counts as failed (`resolver.ErrStale`), so the next source is asked on the same tick and live mode backs off. When everything fails, the status bar says "osada unreachable, showing the cached copy".
4. Use the Kyushu 2026 recordings and the V1 result to adjust the default interval and source priorities.

   **Waiting for data:** the recordings are made during Kyushu 2026 (2026-11-08 to 22, Appendix E). Until then the defaults stay at a 3-minute interval and osada first.
5. Scheduler and request-budget tests (≤ 90 requests per day, mostly 304).

   Done: `TestLiveDay` runs the scheduler and `LivePoll` on a fake clock against a simulated osada. Its cron publishes day 13's results every 3 minutes as the bouts end (Juryo from 14:30, Makuuchi from 16:00, the last at 17:54). The run takes 72 polls from 14:00, and the day is complete at 17:58. One final check follows at 18:28, then no more polls. That is 73 osada requests: the banzuke once and one bout list per poll, 39 of them 304. While results come in every cron run, most answers are new data rather than 304s, so "mostly 304" holds for quiet stretches. Results show within the base interval plus jitter. Only the first ones after the quiet start may take up to the doubled interval (10 minutes at most).

**AC:** during a real tournament day the app shows new results within one poll interval of osada's update, stays within budget, and stops polling once the day is complete.

AC status: met on the simulated day above. The check on a real day happens during Kyushu 2026, together with step 4.

### Phase 11: Spoiler-free mode and `basho watch`

1. `spoiler` package: horizon per tournament and division, saved to the state file, reveal operations, a counter of hidden new results.

   Done, with one change: **one horizon per tournament**, a point in the order of play, rather than one per division. Derived numbers (ratings, last 5, head-to-head, career) run in that one order across both divisions; with two horizons they would either leak the other division's results or stop at the earlier horizon. Juryo fights first each day, so revealing a Makuuchi bout also reveals that day's Juryo, as watching the broadcast does; revealing a Juryo bout keeps that day's Makuuchi hidden. `spoiler.State` loads and saves `state.json` (`{"v":1,"horizons":{"202609":{"day":13,"division":"Makuuchi","match":5}}}`). `Reveal` and `RevealTo` only move the horizon forward; `Set` moves it anywhere (`--reveal-to`). A tournament never revealed starts before Day 1. `derive.Earlier` and `AsOf.After` compare cutoffs.
2. Apply the horizon everywhere:
   - derived cutoffs in the service, including ratings and win chance
   - `?` results in the day view
   - standings and recap locked to the horizon, with a banner
   - picker yusho hidden

   Done: `service.Hiding(ctx, horizons)` limits every cutoff with `derive.Earlier`. That covers records, last 5, kinboshi, head-to-head, movesets, career yusho, ratings and win chance in the tale and the day's rows, standings, recap and the ratings table. Bouts after the horizon come back undecided with `Hidden` set, decided or not: telling decided from undecided would itself be a spoiler. The TUI draws them as `?`. Playoff rows stay out until the horizon has passed the division's last regular bout, since a playoff's existence gives the day away. The yusho and prizes (info, picker, standings) wait until the horizon has passed the whole tournament; the picker says "Yusho hidden (spoiler-free)". Standings and recap carry the banner "as of Day 13 · Makuuchi bout 5 (spoiler-free)", the standings refuse `→` past the horizon's day, and the status bar shows 🙈 spoiler-free. Screens take their service context from `tui.Context.Data()` when they build a command; `tui.SpoilersMsg` makes them reload.
   - **Leak found and fixed:** a day's load only knew that day's bouts, so results of an earlier day cut by the horizon counted by their day. Such a load now also reads the horizon day's bouts (snapshot or cache, one request at most).
3. VS screen `space` reveal flow; `ctrl+r` "reveal up to here" with confirmation; `S` session toggle.

   Done:
   - **VS screen:** shows "Result hidden (spoiler-free) · [space] reveal". `space` moves the horizon through the bout and saves it; every screen on the stack reloads, and the result flashes in once the revealed tale arrives.
   - **Day view:** `ctrl+r` asks "Reveal every result up to Day 13 bout 10? [y/N]". Only `y` reveals; any other key cancels, and while the prompt is open `esc` and `q` answer it rather than leaving.
   - **`S`:** toggles the session's mode (it starts from `spoilers.mode`) and reloads every screen.
   - **Live mode:** in spoiler-free mode it never feeds the day view the poll's answer. The day view reloads behind the horizon from the cache, without `NEW` badges, and the status bar adds "3 new results hidden".
   - **Fix:** screens below the top used to lose the answers to their own commands, because the root delivers to the top. This also affected Phase 10's live updates. The root now addresses those answers back to the screen that asked.
4. CLI `--no-spoilers` and `--reveal-to`.

   Done: both are global flags; `spoilers.mode = "hide"` also hides in the CLI. `--reveal-to` sets the horizon exactly, earlier or later, for the command's tournament (`-t`, otherwise the current one) and saves it. Text output draws hidden bouts as `?` and the tale as "Result hidden (spoiler-free)". JSON adds `"hidden": true` to bouts and tales, with `winner`, `technique` and `result` null. The CLI tests now keep the state file in a temporary directory.
5. `basho watch` line stream (text and NDJSON), reusing the live scheduler.

   Done:
   - **Running:** `basho watch [--division makuuchi|juryo]` runs `live.Scheduler` and `LivePoll` in the terminal. It waits for the window when started early, and exits 3 with the next tournament on a day without one.
   - **Output:** the first poll prints the bouts already decided, Juryo first, as they fought; every later poll prints the newly decided ones. The run ends with "Day complete" or "The live window has closed". Poll failures go to stderr.
   - **`--json`:** one event per line: `{"event":"bout","time":…,"basho":"202609","day":13,"division":"makuuchi","bout":{…the day's bout object…}}`, then `complete` or `window_closed`.
   - **Spoilers:** `watch` ignores spoiler-free mode; watching along is revealing.
6. Tests: no spoiler leaks through any view (snapshot tests with a horizon in the middle of a day), and the horizon persists between runs.

   Done, as a differential test. Every screen is drawn in spoiler-free mode over two worlds that agree up to the horizon and differ after it. One is the real Aki 2026; in the other, every later result went the other way, by a rare technique, with another yusho winner and prizes. Any difference on screen is a leak.
   - **`TestNoLeaks`:** horizon after Makuuchi bout 9 of day 13. It covers days 12–15 in both divisions: day view with the race panel, recap, standings (race, expanded, grid, three popups), every bout's VS screen with the moveset and head-to-head panels, and the picker. All 15 days are loaded first, so later results sit in the bout index and the ratings, where they could leak through last 5, head-to-head and ratings.
   - **Teeth:** taking the horizon out of one record cutoff makes the test fail.
   - **`TestWalkDay13` (the AC):** steps through day 13 with `]` and `space`. Before each `space`, the VS screen equals the one of a world flipped after the current horizon. At bouts 6 and 14 every screen is compared.
   - **Persistence:** the horizon surviving a restart is tested in `spoiler` and with `ctrl+r` in the TUI.
   - **Time:** both tests skip under the race detector; `make check` runs them as `test-leaks`, about 6 s.

**AC:** in spoiler-free mode, a tester can step through day 13 bout by bout with `]` + `space`, and nothing on any screen reveals a later result, including ratings.

AC status: met, by `TestWalkDay13` and `TestNoLeaks` (step 6).

### Phase 12: Polish

1. Light/dark theme check, `--ascii` and `NO_COLOR` passes, mouse wheel support.

   Done:
   - **Themes:** `ui.theme = "dark"` or `"light"` is taken as set; only `auto` asks the terminal for its background. A contrast test holds every text colour to WCAG 4.5:1 and rules and borders to 1.75:1, on white, Solarized Light, Catppuccin Latte, black, VS Code, Ghostty's default (`#282c34`) and Solarized Dark. Ghostty's default background failed it: dark red came out at 4.18 and rules at 1.15, practically invisible. Adjusted colours: dark red `#ff6e64`, dark rules `#545b64`, light dim `#636c76`, gold `#8a5c00`, green `#1a7a35` and rules `#afb8c1`. The STYLE bar categories now share the palette's colours.
   - **`--ascii`:** the glyph set covered only the results. Separators, ellipses, arrows, check marks, bars, borders and `≈` were still Unicode. `asciifold.Fold` maps each to one ASCII cell of the same width (`·` → `.`, `…` → `.`, `✓` → `v`, `│` → `|`, `╭` → `+`). The TUI folds every frame with it, and the CLI folds its text output and `watch` lines; JSON is left as it is. Letters are data and stay: kanji names, places like Töv. `TestASCIIScreens` walks every screen in ASCII mode (days 1, 13 and 15 in both divisions, every VS screen with its panels, standings, recap, picker, help). Without the fold, all 415 screens fail. `make test-leaks` runs it.
   - **`NO_COLOR`:** colorprofile honours only values that `strconv.ParseBool` reads, so `NO_COLOR=yes` kept the colours. Any non-empty value now counts, as no-color.org says, in the TUI and the CLI. The TUI reads the colour profile (`tea.ColorProfileMsg`), and without colours `Theme.Mono` draws the STYLE bar in letters, as `--ascii` does. Auditing everything else that relies on colour alone is step 5.
   - **Mouse:** with `ui.mouse` (the default), the wheel scrolls as `↑` and `↓` do, through the root's key handling, so lists, overlays and focused inputs behave as they do with the keys.
2. Help overlay content: legend, data source notes, credits page, an explanation of the rating, known limitations (historical heya; sekitori bouts only).

   Done:
   - **Pages** (§8.7): Keys, Legend, Rating, Data, Limitations. At 80×24 the old single page cut off the data notes, and a legend line wrapped outside the padding.
   - **Rating page:** takes its numbers from the calibration file through `service.RatingModel`, which reads `params.json` without computing any ratings. It loads in a command when help opens, and the page reads well without the numbers.
   - **Credits:** `service.Credits` and `CreditsNote` are the one copy, shared by the Data page and `basho credits`; they gained "run by a volunteer". `--json` still gives the formatted text.
   - **Wrapping:** `ansi.Wordwrap` always breaks after a hyphen (head-/to-head, sumo-/api), so text wraps with non-breaking hyphens that are swapped back afterwards.
   - **Shared:** the STYLE categories and their letters now live in `components` only (the VS screen kept a copy).
3. `basho config path|show`; friendlier first-run messages.

   Done:
   - **`basho config`** (§12): `path` and `show`, both with `--json` (`config_path`, `config`). `config.Config.Encode` writes the file's layout, which `Load` reads back. Durations print as the file writes them (`3m`, not `3m0s`), in `show` and in error messages.
   - **Error box:** fang capitalised the first word of every error. A message starting with a path came out as `/Private/Tmp/…`, a path that doesn't exist, and fang's hard wrap split paths at hyphens. `errorHandler` keeps fang's look without either, so paths can be copied.
   - **Hints:** each failure says what to do, on its own line.
     - **Config errors** name where settings come from: the file and `BASHO_*` variables. Validation errors start with `config:`.
     - **A broken state file** says it only keeps reveal horizons and can be deleted.
     - **Offline without a copy** drops the repeated URL and says to run once without `--offline`.
     - **Every source failed:** check the connection; the snapshot's tournaments work offline.
     - **TUI:** the day, VS and standings screens show `tui.ErrorHint` as a dim line under the error. `service.Offline` tells an offline miss from an outage, and the TUI now says which it is.
4. Performance: hoshitori grid renders in under 16 ms at 130×50; snapshot and bout index load fast enough for an instant start (target under 300 ms to first screen).

   Done: both budgets hold without changes; `make bench` measures them.
   - **First screen:** 106 ms from launch until the day view's bouts are on screen, for the real binary in a 130×50 pseudo-terminal (Apple M5), the first frame after 21 ms. `BenchmarkStart` measures 96 ms in-process: the snapshot opened, the current tournament's latest day read and drawn.
   - **Grid:** `BenchmarkGrid` draws the whole frame with the Makuuchi grid after day 15 at 130×50 in 1.3 ms.
   - **Memory:** peak RSS is about 150 MB, also for `basho day`. It comes from decoding every tournament into the bout index (about 195,000 bouts) and replaying the ratings. Pre-sizing the per-tournament day index saved 4% of the allocations and nothing in RSS, so it was left out.
5. Accessibility: never rely on colour alone (glyphs and KK/MK text always shown).

   Done: every use of a meaningful colour was checked.
   - **Already fine:** results, records, kinboshi and rating changes carry glyphs or text (○ ● ★ ▲ ▼, KK/MK), and the live status a glyph and words. Every list marks its selection with ▶ or brackets. Leaders, wrestlers out of contention and the current window differ by weight (bold, faint), which `NO_COLOR` keeps.
   - **STYLE bar:** with colours on, all five categories were the same █, told apart by hue alone, and red and green are belt and pull. Each category now has a texture too (PUSH █, BELT ▓, THROW ▒, PULL ░, OTHER ▚), as the §8.5 mockup drew them; without colours or in ASCII, letters (step 1). The help legend uses the same marks.
   - **Help tabs:** the current page is in brackets, like the day bar's `[13]`.
6. VHS tape and GIF for the README.

   Partly done: `docs/demo.tape` and `make vhs` (needs VHS) are in. The GIF is recorded on the owner's machine, and the README embeds it in the same commit. The tape runs `bin/basho --offline` on its own XDG directories, opens Aki 2026 through the picker behind `Hide`, and shows the following, about 35 s:
   - day 13 with the yusho race
   - the musubi-no-ichiban's VS screen, its moveset, and the bout before
   - the standings race and grid
   - spoiler-free mode on and off
   - two help pages

   The key sequence was checked against the app in the test driver.

**AC:** a full manual test pass (checklist in `docs/qa.md`) in Ghostty and one other terminal, in light and dark themes.

AC status: moved to the QA pass (Phase 16), which runs after the phases added before v1.0. The checklist is written (`docs/qa.md`).

**Before v1.0** (reordered 2026-10-15): the items that were optional or "not in v1" — the rating scale, `ui.names`, portraits and the wrestler screen — come first. Then the QA pass covers everything at once, and the release is last.

### Phase 13: Rating scale and name display

1. **Calibrated win chance** (§6.8, §15): plain Elo is overconfident for lopsided bouts (V9).

   Done: investigated and dropped (no code change). The fit, built as planned, comes out at 406 on the training years: displayed chances would move by less than 0.3 points, and evaluation log loss from 0.6772 to 0.6770. Fitted era by era, the divisor is 380–434 everywhere except 2015–2020 (562); for 2021–2026, the years shown most, 400 already scores 0.6771 against 0.6770 at the best divisor (428), and 562 would make it worse (0.6779). The overconfidence is a drift in one era, not a constant (V9, §15).

   The original plan:
   - Calibration fits a divisor `s` for the win chance `1 / (1 + 10^(−d/s))`, once the grid has chosen `K`, `K_new` and `r`. It is a one-dimensional search for the lowest log loss on the training years.
   - The ratings and their updates keep 400. Only the chance uses `s`, wherever it shows: VS screen, day view column, recap upsets, CLI and JSON.
   - `params.json` gains `scale`. `docs/rating.md` shows the calibration table before and after, V9 is checked again, and so is the 35% upset threshold.
2. **`ui.names`** (D5, §12), no longer hidden:
   - `both` (default): romaji first, kanji alongside, as now.
   - `en`: no kanji anywhere, on every screen and in the CLI's text output.
   - `jp`: kanji first, romaji alongside; romaji alone where no kanji is known.

   JSON always carries both.

   Done:
   - **Where:** `domain.Name.Show` and `Alt` hold the rule. The TUI reaches it through the theme (`Theme.Name`, `AltName`, `Kanji`, set by `WithNames`), the CLI through its glyphs. All 39 places that printed `Romaji()` go through it.
   - **`en`:** besides the names, it drops the techniques' kanji (VS result panel, recap, `basho bout`) and turns 結び and 休 into "musubi" and "kyujo", as in ASCII mode.
   - **`jp`:** kanji names are two cells per character, where the day table measured bytes and fmt pads by runes. Names are now measured and padded by width (`tui.PadRight`). The VS screen's second line holds the romaji, and is left out when neither side has anything for it.
   - **JSON:** unchanged.
   - **Kept:** `Theme.Rebuild` keeps the mode, and mono, when the terminal reports its background.
3. Tests: goldens in the three name modes, and the CLI.

   Done:
   - **`TestNameModes`** walks every screen. In `en`, days 1, 13 and 15 must show no kanji or kana; in `jp`, day 13 must lead with kanji, keep the romaji on the VS screen, and fit 140 columns. `make test-leaks` runs it.
   - **Goldens:** day 13 and the musubi-no-ichiban in `en` and `jp`, at 80×24 and 120×40.
   - **CLI:** `bout` and `day` in all three modes.

**AC:** with `ui.names = en`, a walk over every screen finds no kanji. (The first half, a better calibrated chance, fell with step 1.)

AC status: met, by `TestNameModes`.

### Phase 14: Portraits

1. `basho-data portraits update`: Wikidata query (P3385 → P18), Commons metadata and licence allow-list, polite thumbnail download.

   Done (the fetch stage; steps 2 and 3 turn its output into `data/portraits/`):
   - **Command:** `basho-data portraits update [--only IDs] [--limit N] [--dry-run] [--contact …]`, and `make portraits` (dry run, confirm, run; `CONTACT=` sets the contact).
   - **Requests:** one SPARQL query, the Commons metadata 50 files at a time, then one 500 px thumbnail each. Every host gets 1 request per second, one at a time. The work cache keeps the query and metadata a week and the thumbnails for good, so a rerun fetches nothing.
   - **Join:** by SumoDB ID (P6652) first, then by JSA ID. The JSA ID alone found 88 wrestlers; both find 360 (§4.4).
   - **Licences:** checked before any download: CC0, public domain, and CC BY and BY-SA 2.0–4.0 including ported versions (`cc-by-sa-2.1-jp`). Each thumbnail must decode.
   - **Output:** in `--work`: `images/<id>.<format>`, `sources.json` (item, file, licence and its URL, author and date as plain text, year, source page) and `skipped.json` with reasons.
   - **User-Agent:** `basho-data/<version> (<contact>)`. Wikimedia asks for a contact; the default is the repository URL, and `--contact` takes an email.
   - **Found on the smoke run** (3 wrestlers, 5 requests): thumbnails come from `thumb.wikimedia.org`, which now gets the same limit and caching.
2. Face crop with `pigo` and the `overrides.toml` mechanism; downscale to 32 and 20 px; palette quantization with optional dithering.

   Done:
   - **Crop:** pigo's face cascade (copied into `internal/datatool/cascade/` with its MIT origin noted; `basho-data` only) finds faces with confidence 5 or more, and the largest one counts. The crop is a square of 2.2 face widths with the face's centre 40% from the top, moved, and if need be shrunk, to stay inside the photo. Without a face, it is a top-centred square, and the run lists the wrestler for review.
   - **Overrides:** `data/portraits/overrides.toml` has `[[portrait]]` entries: `id`, `crop = [x0, y0, x1, y1]` in the thumbnail's pixels, `exclude`, `file` (another Commons file) and `note`. Bad crops, duplicates and unknown keys are errors. The fetch honours `exclude` and `file`.
   - **Scale:** our own area averaging: each pixel is the mean of the area it covers, partly covered pixels by their share. No `x/image` dependency.
   - **Palette:** DB32, mapped by the nearest colour in CIELAB. The first try used the redmean RGB distance, which turned shaded skin into the palette's greys and blues. `--dither` adds 4×4 ordered dithering; it is off by default, and the review decides.
   - **Output:** `work/out/<id>-32.png` and `-20.png`, and `work/sheet.png` with every crop next to its portrait enlarged 4 times, for the review (step 6).
3. `data/portraits/` with PNGs, `credits.json`, generated `CREDITS.md` and `LICENSE`; `basho credits` and the help overlay's Data page.

   Done:
   - **Files:** `basho-data portraits update` writes `data/portraits/32/<id>.png` and `20/<id>.png` (paletted PNGs), `credits.json`, `CREDITS.md` (a table of every photo with author, licence and source) and `LICENSE` (the images keep their photos' licences; the code's licence is separate). It removes the PNGs of an earlier run and keeps `overrides.toml`. A run with `--only` or `--limit` never writes there.
   - **Shipped:** the binary embeds `data/portraits`. Until the owner's full run, `credits.json` is `[]` and there are no images.
   - **Access:** the `portrait` package reads the set (`Open`, `Credits`, `Credit`, `Image`). The service exposes it as `PortraitCredits` and `Portrait(id, size)`, loaded once like the snapshot.
   - **`basho credits`:** a portraits line with the count when there are photos, and the full list with `--portraits`, each photo with year, file, author, licence and source. JSON gains `portraits`. `credits` now goes through the service like the data commands.
   - **Elsewhere:** the help overlay's Data page gets the same line, loaded with the rating numbers when help opens, and the README credits the photos.
4. Half-block renderer with mirroring and the 256-colour fallback; procedural silhouette for wrestlers without a photo.

   Done (`portrait.Render`, `portrait.Silhouette`):
   - **Cells:** ▀ with the upper pixel as foreground and the lower as background. A transparent pixel shows the terminal: ▄ when only the lower pixel is drawn, a space when neither is. An odd last row draws only its upper half.
   - **Mirroring** flips the image for the West wrestler.
   - **256 colours:** the renderer writes true colour, and Bubble Tea's screen converts every cell to the terminal's profile (`SetColorProfile`), 256 colours and fewer included. Nothing of our own was needed.
   - **Silhouette:** a head with a topknot over broad shoulders, drawn at any size in one colour on a transparent ground, and symmetric, so mirroring leaves it as it is.
   - **Speed:** a 32 px portrait renders in 0.5 ms (`BenchmarkRender`).
5. VS screen integration at the size breakpoints (§8.9), photo-year caption, `ui.portraits` toggle.

   Done:
   - **Placement:** first beside the tape, East on the left facing right, West mirrored on the right, each with "photo 2024" under it when the year is known. Since 2026-10-15 they are above the names instead (§8.9), where they cost no width.
   - **Sizes:** chosen by width, as §8.9 now says. Not shown with `ui.portraits = false`, `--ascii`, without colours, or in the compact layout.
   - **Without a photo:** a silhouette in the rules' colour.
   - **Caching:** each portrait's text is rendered once and kept for the screen, since the VS screen redraws on every animation frame.
   - **Tests:** goldens at 200×50 (32 px) and 125×40 (20 px) use a generated test image, so no photo goes into the repository. Every gate and breakpoint is tested.
6. Manual review of all generated portraits for current Makuuchi and Juryo wrestlers, and fix crops via overrides.

   - **Look, after the first full run:** with DB32's 32 colours the photos turned to soup, the old sepia and black-and-white ones especially, and the wrestlers were hard to tell apart. A comparison over the cached thumbnails showed that the palette was the main loss, then the crop, then the size. Now each portrait gets a palette of its own 16 colours (median cut; `--colours` 2–256 tries others from the cache), the crop is 1.7 face widths with the face 45% down, and a 48 px size joined 32 and 20 for wide terminals; it went again when the portraits moved above the names. It stays pixel art, with crisp blocks and few colours, and DB32 is gone.
   - **Progress** (after the first full run, which said nothing for 6 minutes): each phase reports its count, metadata 3/8, thumbnails 37/360, portraits 120/360, on one rewritten line in a terminal and every 25 items in a log. A rerun from the work cache takes about 30 s, mostly face detection.

   - **Review tools:** each cell of `sheet.png` has a label with the ID and name, marked "(override)" or "(no face)". `basho-data portraits update --overrides-only`, or `make portraits-preview`, rebuilds only the wrestlers in `overrides.toml`, into `.cache/basho-data/portraits-preview/sheet.png`, from the work cache in seconds. It never writes `data/portraits`.
   - **First review** (2026-10-15, all 360): `overrides.toml` excludes 7 images (three handprints, a statue, an arena wide shot, and two action shots without a usable face) and crops 68 by hand. Of those, 35 were full-body or profile shots where no face was found; the others caught a spectator, a towel, the belly, an apron or a banner. Each crop was checked with the preview, and the coordinates were read off the thumbnails with a 50 px grid.

   Done: the owner's full run with the reviewed overrides is committed. That is 353 wrestlers, 360 found less 7 excluded, at 32 and 20 px, 294 KB. Dithering stays off.
7. Golden tests (§13).

   Done with steps 4 and 5: the renderer and silhouette tests, and the VS screen goldens with portraits at 200×50 and 125×40 from a generated image.

**AC:** portraits render for every current Makuuchi wrestler with a Commons photo; every image is correctly credited; the VS screen still works at every size with portraits on and off.

AC status: met.
   - **Coverage:** 35 of the 42 Makuuchi wrestlers of Aki 2026 have a portrait, every one with an image on Wikidata (the same 35 as counted on 2026-10-13). The other seven, mostly recent promotions, have none linked: Asasuiryu, Wakanosho, Shonannoumi, Kotoeiho, Fujiryoga, Asahakuryu and Toshinofuji. A Commons photo found by hand can be added with `file =` in `overrides.toml`. Juryo has 17 of 28.
   - **Credits:** every image is credited in `CREDITS.md`, `credits.json` and `basho credits --portraits`.
   - **VS screen:** tested at every size with portraits on and off.

### Phase 15: Wrestler screen

1. **Service:** `service.Wrestler`, a wrestler's career as of a cutoff (the spoiler horizon included):
   - the profile
   - every sekitori tournament with rank, division, record, yusho and special prizes
   - the rating after each tournament
   - kinboshi
   - the technique profile over the career and the last 6 tournaments (§6.7)

   Done (§8.10):
   - **The cutoff** is a tournament, `through`: a career opened from a bout of Aki 2024 must not tell what came after it. The CLI defaults to the latest tournament.
   - **Records:** the index keeps each wrestler's placements, rank, shikona and day outcomes, from the pass that builds it. Records are counted to the cutoff from them (`derive.RecordThrough`), and tournaments after the snapshot come from the remote banzuke.
   - **Spot checks:** Hakuho matches his official Makuuchi record, 1093–199–253, with 45 yusho. Taiho has 32 yusho, but his record is a win and a loss short because of the two days missing from the source. Onosato's career ends at Aki 2026 with its yusho, 12–3. In spoiler-free mode with the horizon after Makuuchi bout 5 of day 13, that tournament stops after 12 days, its yusho stays unknown, and the rating freezes.

2. **TUI** (`tui/wrestler`):
   - a header with the names, heya, hometown, age, height, weight and the portrait (Phase 14)
   - sparklines of rank and rating over the career
   - the technique profile (STYLE bar, signature, top wins and losses)
   - a table of every tournament, scrolled with `j`/`k` and the wheel; `enter` opens that tournament's day view on the wrestler's last bout

   It opens from the VS screen (either side) and from the standings grid's results popup; the keys go into §8.8 in this step.

   Done (§8.10, §8.8):
   - **Keys** (the owner chose): `1`/`2` on the VS screen, and `i` in the standings grid and its popup.
   - **Layout** (the owner chose): header with the portrait, trends, then the table.
   - **Opening a day:** the day view's package imports the VS screen, which imports this one, so the root context carries `OpenDay`, set by `cmd/basho`; `day.NewIn` opens a division. `CareerBasho.LastDay` places the day.
   - **Bug found:** for a wrestler without a profile, the yusho and prize totals were always 0, because `CareerAt` matched them by the empty profile's ID 0.
   - **Fixed after a screenshot review:** the header's rank was cut beside the portrait, the "last N of M" note wrapped, and the table pushed the hints off the screen.
   - **`--ascii`:** the sparkline heights fold to a `_ . - = #` ramp.
   - **Tests:** goldens at 80×24, 120×40 and 160×50 over the fixture snapshot with a generated portrait, navigation and `enter`, and the keys on the VS and standings screens.
3. **CLI:** `basho wrestler NAME|ID`, text and `--json`. NAME is a shikona from any era, matched exactly and ignoring case. A name several wrestlers used lists them and exits 2. This is not a search screen (§1.2).

   Done:
   - **Lookup** (`service.FindWrestlers`): every shikona in the placements, in any case, and today's name in romaji or kanji; a number is an ID. Several wrestlers with the name, Takanohana for one (father and son), list their IDs, first and last years and last rank, and exit 2. An unknown name exits 3.
   - **Output:** text shows the profile, totals and fighting style, then the tournaments newest first, as on the screen. JSON has kind `wrestler` (docs/json.md).
   - **`-t`:** sets the tournament the career is seen from, `current` by default; spoiler-free mode cuts it at that tournament's horizon.
   - **Kanji:** sumo-api has none for many older wrestlers (Hakuho among them), so they are found by romaji only.
   - **Shared:** the moveset JSON moved into `movesetJSON`, which the tale's JSON uses too.
4. **Spoiler-free:** everything stops at the horizon, ratings included; `TestNoLeaks` and `TestASCIIScreens` walk the new screen.

   Done: the shared walker opens both wrestlers' careers from VS bouts 1, 10 and 19 of each day walked, and the grid wrestler's with `i`. `TestNoLeaks`, `TestASCIIScreens` and `TestNameModes` all cover the screen now, together about 7 s. Teeth: without the horizon in `service.Wrestler`, 38 career screens leak.
5. Tests: goldens at the three widths, the CLI, and spot checks against the snapshot (Taiho, Hakuho, Onosato).

   Done with steps 1–3:
   - **Spot checks:** Hakuho's official record, Taiho's 32 yusho, Onosato's Aki 2026.
   - **Goldens:** the screen at 80×24, 120×40 and 160×50; the CLI's text and JSON.
   - **Keys and navigation**, and the name lookup, `RecordThrough` and the horizon.

**AC:** both wrestlers' careers open from any VS screen; their rank and rating history match the snapshot and the rating replay; no leak in spoiler-free mode.

AC status: met.
   - **Opening:** `1`/`2` on every VS screen (tested, and walked across the leak test's days).
   - **History:** ranks and records match the snapshot (Hakuho to the official record), and ratings come from the replay at each tournament's end.
   - **Spoilers:** no leak in `TestNoLeaks`.

### Phase 16: QA pass

1. Extend `docs/qa.md` with Phases 13–15.

   Done:
   - **New sections:** the wrestler screen, `ui.names`, and the data tools, checked once rather than per run.
   - **Changed since Phase 12:** VS screen items for the portraits above the names (alignment, the 40-row threshold, no flicker), plain height and weight, and `1`/`2`; `i` in the standings; `basho wrestler` and `credits --portraits`; the help's Keys and Data pages.
   - **Run table:** gains the terminal's size, since portraits depend on it.
2. The owner's four runs: Ghostty and one other terminal, light and dark. Live mode is checked during a tournament if one falls in the window.
3. Findings are fixed one commit each and written into the spec.
4. Record the README GIF (`make vhs`) and embed it in the README.

   - **Portraits off:** `ui.portraits = false` hid them from the start. It now has a session flag, `--no-portraits`, and the help's Data page names both, since nothing pointed to the setting before.

   Prepared: the tape now also shows Onosato's career page (`1` on the VS screen, a scroll down the table, `esc`), about 5 s more. Recording and the README embed wait for the owner's run.

**AC:** Phase 12's: every item in `docs/qa.md` holds in all four runs, and no finding is open.

### Phase 17: v1.0 release (private)

1. `make install` with the ldflags; `basho version` shows the snapshot date from the manifest.
2. `.goreleaser.yaml`: targets, ldflags, archives with completions and man page, checksums. No Homebrew section yet.
3. Dry run with `make release-snapshot`.
4. Document the snapshot release flow (update → verify → commit → patch tag → install) in `docs/releasing.md`; CHANGELOG.
5. Tag `v1.0.0`.

**AC:** `make install` produces a `basho` whose `basho version` shows the tag and the snapshot date; zsh completions work; a snapshot-only patch release takes under 15 minutes of manual work.

---

**After 1.0:**

### Phase 18: Public repository and Homebrew tap

1. Decide to go public (D9). Review the history, README, LICENSE for the code and the data credits beforehand.
2. Add `homebrew_casks` to `.goreleaser.yaml` (§14.3) with the quarantine `postflight` hook; release locally with `goreleaser release` (§14.3).
3. Release, then check `brew tap … && brew install --cask thorstenpfister/basho/basho` on a clean user account.
4. Add the tap and cask to the Brewfile in `macos-setup`.

**AC:** `brew install` works on Apple Silicon without manual Gatekeeper steps; `basho version` matches the tag.

---

*Next step: mark this spec v1.0 after the final read.*

---

## Appendix A: Observed API responses (2026-10-13)

Recorded by hand while researching. These are facts to rely on until the fixtures exist; the fixtures then take over.

### A.1 sumo-api.com

- Server: nginx; `Access-Control-Allow-Origin: *`; **no** `ETag`, `Last-Modified` or rate-limit headers seen.
- `GET /basho/202609`:
  ```json
  {"date":"202609","startDate":"2026-09-13T00:00:00Z","endDate":"2026-09-27T00:00:00Z",
   "yusho":[{"type":"Makuuchi","rikishiId":8850,"shikonaEn":"Onosato","shikonaJp":"大の里　泰輝"}, …6 divisions],
   "specialPrizes":[{"type":"Kanto-sho","rikishiId":615,"shikonaEn":"Fujinokawa","shikonaJp":"藤ノ川　成剛"}, …]}
  ```
  - No `location` for recent tournaments; `196001` has `"location":"Tokyo, Kuramae Kokugikan"`.
  - Unknown or future tournament (`202611` today, `195801`): `{"date":"","startDate":"0001-01-01T00:00:00Z","endDate":"0001-01-01T00:00:00Z"}`, with HTTP 200.
- `GET /basho/202609/banzuke/Makuuchi` → `{"bashoId","division","east":[…],"west":[…]}`. Each entry:
  ```json
  {"side":"East","rikishiID":8850,"shikonaEn":"Onosato","shikonaJp":"大の里　泰輝","rankValue":101,
   "rank":"Yokozuna 1 East","wins":12,"losses":3,"absences":0,
   "record":[{"result":"win","opponentShikonaEn":"Daieisho","opponentShikonaJp":"大栄翔","opponentID":9,"kimarite":"hatakikomi"}, …15]}
  ```
  - Note `rikishiID` (capital ID) here, but `rikishiId` in `/basho`.
  - `record` always has 15 entries. `result` ∈ `win`, `loss`, `fusen win`, `fusen loss`, `absent`. `kimarite` is `fusen` for fusen and `""` for absent.
  - 202609 Makuuchi: 22 East, 20 West entries. A wrestler absent for all 15 days still appears (e.g. Hoshoryu, 0-0-15).
  - Partial absence example: Kotoshoho `L L L W W L W L W W L fusen-loss A A A` → 5-7-3.
  - 196001: `shikonaJp` empty for opponents; `shikonaEn` may include the given name in `/basho` yusho ("Tochinishiki Kiyotaka") but not in the banzuke ("Tochinishiki").
- `GET /basho/202609/torikumi/Makuuchi/15` → the `/basho` object plus `"torikumi":[…]`. Each bout:
  ```json
  {"id":"202609-15-1-49-8865","bashoId":"202609","division":"Makuuchi","day":15,"matchNo":1,
   "eastId":49,"eastShikona":"Shonannoumi","eastRank":"Maegashira 17 East",
   "westId":8865,"westShikona":"Kazuma","westRank":"Juryo 2 West",
   "kimarite":"oshidashi","winnerId":8865,"winnerEn":"Kazuma","winnerJp":"一意　虎風"}
  ```
  - Counts: 202609 Makuuchi day 15: 19; Juryo day 1: 14; 202607 Makuuchi day 15: 20 (playoff? V2); 196001 Makuuchi day 1: 22.
  - Day 16: empty `torikumi`.
- `GET /rikishis?intai=true&limit=1000&skip=0` → `{"limit":1000,"skip":0,"total":9132,"records":[…]}`. Without `intai=true`: `total` 601 (active only). Each record:
  ```json
  {"id":2,"sumodbId":12291,"nskId":3682,"shikonaEn":"Asanoyama","shikonaJp":"朝乃山　広暉",
   "currentRank":"Maegashira 6 East","heya":"Takasago","birthDate":"1994-03-01T00:00:00Z",
   "shusshin":"Toyama-ken, Toyama-shi","height":188,"weight":172,"debut":"201603",
   "updatedAt":"2026-09-17T09:17:34.058Z"}
  ```
- `GET /rikishi/8850?measurements=true&ranks=true&shikonas=true` adds `rankHistory` (21 entries), `shikonaHistory` (2), `measurementHistory` (1).
- `GET /rikishi/8850/matches` → `{"limit","skip","total":276,"records":[…]}` with the full career, newest first, **without** passing `limit`. The records have the torikumi shape, including `eastRank`/`westRank`.
- `GET /rikishi/8850/matches/9` (head-to-head) → `{"kimariteLosses":{"yorikiri":1},"kimariteWins":{"hatakikomi":6,"oshidashi":3,"abisetaoshi":1},"opponentWins":1,"rikishiWins":10,"total":11, "matches":[…]}`.
- `GET /rikishi/8850/stats` → keys `absenceByDivision`, `basho`, `bashoByDivision`, `lossByDivision`, `sansho` (map by prize), `totalAbsences`, `totalByDivision`, `totalLosses`, `totalMatches`, `totalWins`, `winsByDivision`, `yusho`, `yushoByDivision`.
- `GET /measurements?bashoId=202609` → array of `{"id":"202609-9144","bashoId":"202609","rikishiId":9144,"height":177,"weight":124}`, but **only the 7 newcomers** of that tournament; `196001` → 1 entry. `measurementHistory` of a profile has one entry, from the debut. Not used.
- `GET /kimarite` without parameters → `{"error":"NOT_A_VALID_KIMARITE_SORT_FIELD,_CHOOSE_FROM_[COUNT,_KIMARITE,_LASTUSAGE]"}`.

### A.2 osada.us

- Cloudflare; `cache-control: public, max-age=0, must-revalidate`; **`ETag` present** (e.g. `"afd40f351a66be623651ef28d43e798d"`).
- `GET /api/v1/torikumi.json` top level: `bashoId` (**a serial number, not YYYYMM**; the month comes from the first 6 digits of `pathDate`), `bashoName` (`九月場所`), `year` (`令和八年`), `updatedAt`, `resultUpdatedAt`, `scheduleUpdatedAt` (ISO with `+09:00`), `today`, `tomorrow`, `resultDays[]`, `scheduleDays[]`.
- Day entry: `{"day":15,"isoDate":"2026-09-27","pathDate":"20260927","label":"千秋楽","dayHead":"千秋楽： 令和8年9月27日(日)","status":"published","statusMessage":null,"data":{"makuuchi":{…},"juryo":{…}}}`.
- Division block: `{"day","dayName","dayHead","division":"幕内","matches":[…],"absentees":[…]}`.
- Match:
  ```json
  {"division":"幕内","boutNo":1,"eastName":"しょうなんのうみ","eastYomi":"","eastEnglish":"",
   "eastRank":"前頭十七枚目","eastProfileUrl":"https://www.sumo.or.jp/ResultRikishiData/profile/3553/",
   "westName":"かずま", …, "westRank":"十両二枚目", "kimarite":"押し出し","winner":"west"}
  ```
  - `winner` ∈ `east`, `west`, `null`. `kimarite` `未定` means undecided. Playoff bouts carry `"isPlayoff":true` and were still `未定` in `scheduleDays` after the tournament (V6).
  - The names in bouts are **hiragana**; the English fields were empty.
- `GET /api/v1/banzuke.json`: `bashoName`, `year`, `updatedAt`, `makuuchi:[{title, east:[…], west:[…]}]`, `juryo:[…]`. Wrestler:
  ```json
  {"id":4227,"name":"大の里","yomi":"おおのさと","rank":"横綱","side":"east","wins":12,"losses":3,"draws":0,
   "results":["win", …15],"profileUrl":"https://www.sumo.or.jp/ResultRikishiData/profile/4227/","memo":""}
  ```
- Last September update stamp: `2026-09-27T17:27:38+09:00`, with all 19 Makuuchi results of day 15 present.

---

## Appendix B: Kimarite table

The table lives in `internal/domain/kimarite.go`, which is the single source: key, kanji, English gloss and category for every technique. Tests check that it has the 82 official techniques, that every kanji is unique, and that it covers every value sumo-api.com uses.

- **Key:** sumo-api's spelling, lower case, no spaces or hyphens (`NormalizeKimarite`).
- **Category:** basho's own grouping (§6.7, D17), not the JSA's:
  - **PUSH:** pushing and thrusting
  - **BELT:** belt fighting, forcing out or down
  - **THROW:** throws, twists, trips and backward body drops
  - **PULL:** pulling and slapping down
  - **OTHER:** rare techniques and the opponent's own mistakes (the five *hiwaza*, `hansoku`, `ushiromotare`)
  - **excluded:** `fusen` and the old-era draws `hikiwake` and `itamiwake`. They are not wins by technique and are left out of profiles.
- **Contents:** the 82 official techniques, plus the 5 non-techniques, `hansoku`, `fusen`, the two draws, and two historical spellings sumo-api still uses (`dashinage`, `yobikaeshi`) (V11).
- **Unknown keys** keep their raw value, display as-is and count as OTHER.
- **Japanese names:** they map through `KimariteByKanji` (exact match). Source-specific cleanup, such as osada's `決まり手：` prefix and full-width spaces, happens in the osada adapter.

---

## Appendix C: Ranks, venues, calendar facts

**English rank strings (sumo-api):** `<Title> <N> <East|West>`, for example `Yokozuna 1 East`, `Maegashira 17 East`, `Juryo 2 West`. Titles in scope: Yokozuna, Ozeki, Sekiwake, Komusubi, Maegashira, Juryo. `rankValue` sorts the banzuke (V10).

**Japanese rank strings (osada):**

| String | Meaning |
|---|---|
| `横綱`, `大関`, `関脇`, `小結` | san'yaku titles. In `banzuke.json` without a number; the number follows from the order within the title group and side. |
| `前頭N枚目` | Maegashira N |
| `十両N枚目` | Juryo N |
| `前頭筆頭`, `十両筆頭` | **筆頭 (hittō) means number 1** |
| `張出…` | haridashi (outside the regular positions): keep the title and map to the next number within title and side, no flag (sumo-api does the same, V10) |

- Kanji numerals: 一 二 三 四 五 六 七 八 九 十, compounds such as 十七 = 17 and 二十 = 20.

**Venue by month** (displayed when the API has no location; years to verify in Phase 1):

| Month | City | Venue |
|---|---|---|
| Jan, May, Sep | Tokyo | Kuramae Kokugikan until 1984; Ryogoku Kokugikan from 1985 |
| Mar | Osaka | Osaka Prefectural Gymnasium (now Edion Arena Osaka) |
| Jul | Nagoya | Aichi Prefectural Gymnasium; IG Arena from 2025 |
| Nov | Fukuoka | Fukuoka Kokusai Center (Fukuoka Sports Center before 1981) |

If a year boundary is uncertain, show the city only.

**Calendar facts:**

- 15 days per tournament throughout the whole range (since 1949).
- Cancelled: 2011-03 (match-fixing scandal), 2020-05 (COVID-19).
- Special: 2011-05 was a "technical examination tournament" without spectators. It is not an official honbasho; whether sumo-api has data for it is checked in Phase 1.
- 2020-03 was held behind closed doors; it is a normal tournament for our purposes.
- Banzuke publication is about 13 days before day 1. The bout lists for days 1 and 2 are published together, about 2 days before the start.

---

## Appendix D: Snapshot file schema (schema version 1, as implemented)

Owned by `internal/snapshot` (`manifest.go`, `tournament.go`, `rikishi.go`). Every file carries the schema version; a reader refuses others. Encoding is deterministic.

`basho/YYYYMM.json.zst` (decompressed, shortened; real values from Aki 2026):

```json
{
  "v": 1, "id": "202609", "start": "2026-09-13", "end": "2026-09-27",
  "yusho": {"Juryo": 66, "Makushita": 8860, "Makuuchi": 8850},
  "prizes": [{"type": "Kanto-sho", "id": 615}, {"type": "Kanto-sho", "id": 71}],
  "banzuke": {
    "Makuuchi": [{"id": 8850, "en": "Onosato", "jp": "大の里", "rank": "Yokozuna 1 East", "rec": "WWWLWWWWWWWWLLW"}],
    "Juryo": []
  },
  "bouts": [
    {"div": "Makuuchi", "d": 1, "n": 1, "e": {"id": 49}, "w": {"id": 135, "en": "Dewanoryu", "r": "Juryo 1 East"}, "win": 2, "k": "yorikiri"}
  ]
}
```

- `location` is omitted when the source has none; `yusho` holds every division the source names, `prizes` the special prizes.
- `rec`: one letter per day, by `domain.Outcome`: `.` pending, `W` win, `L` loss, `F` fusen win, `f` fusen loss, `A` absent, `D` draw, `U` unknown. Opponent and kimarite of a day come from that day's regular bout; only days where the record differs are written in `"x": [{"d": day, "o": opponent, "k": kimarite}]`.
- Bouts are sorted by day, division, playoff (`"p": true`) last, then match number. `win`: 1 east, 2 west, omitted while undecided. A side that is only `{"id"}` has the banzuke's romaji name and rank and no kanji or kana (what sumo-api's bout lists carry); anything else, such as a Makushita visitor in a Juryo bout, is written out (`en`, `jp`, `kana`, `r`).
- Ranks are the long form (`"Maegashira 17 East"`), divisions and outcomes their names (`domain` text marshalling).
- `rikishi.json.zst`: `{"v":1,"rikishi":[{"id","nsk","sumodb","en","jp","kana","heya","shusshin","birth","debut","cm","kg","retired"}]}`, sorted by id, only wrestlers who appear in the snapshot.
- `manifest.json` (indented, plain JSON): `{"schema":1,"generator":"<basho-data version>","generatedAt":"…","source":"sumo-api.com","basho":["196001",…],"indexThrough":"<newest tournament when the index was written; cleared when a tournament is added>","files":{"basho/196001.json.zst":"<sha256>",…}}`.
- `known-issues.txt` (optional, not in the manifest): accepted source oddities for `verify`, one exact problem line each after a `#` reason.
- Measured: Aki 2026, 507 bouts, 6,976 bytes; the full snapshot is 2.50 MB (V8).

---

## Appendix E: Live recorder for Kyushu 2026

`basho-data record [--out DIR] [--interval 10m] [--from 14:00] [--until 19:15] [--tournament YYYYMM] [--once]`

- **When it runs:** only on tournament days, between `--from` and `--until` (JST). On other days it logs one line and exits, so launchd can start it every day.
- **Which tournament:** the tournament is this month's, or the upcoming one in an even month. Its start date comes from sumo-api `/basho/{id}`, falling back to the second-Sunday estimate.
- **What each tick saves** into `DIR/<YYYYMMDD>/<HHMM>/`:
  - Files are named like the fixtures (`fixture.Name`, under `osada/` and `sumoapi/`), so a tick can be replayed with `fixture.Getter` in tests.
  - osada `torikumi.json` and `banzuke.json`, fetched with a conditional GET. A 304 is recorded with its status, and the body file repeats the last 200, so every tick folder is self-contained.
  - sumo-api `/basho/{id}`, plus `/basho/{id}/torikumi/{Makuuchi|Juryo}/{day}`
  - `meta.json`: request times, status codes, ETags, sizes, durations and errors
- **Volume:** 5 requests per tick, 32 ticks a day (about 160 requests), within the §7.1 limits.
- **Robustness:** a tick that is less than half an interval late still runs. Failed requests are recorded, not fatal. Files are written atomically, and SIGTERM stops the run between files.
- **`--once`:** records one tick immediately, ignoring the window and the tournament days. This is the installation test.
- **Where it runs:** unattended via launchd; see `docs/runbooks/kyushu-recorder.md` and `deploy/launchd/`.
- **Later:** `basho-data record analyze` compares when each result first appears in osada vs sumo-api (V1) and writes `docs/v1-latency.md`. It is built after the tournament.
- **Committing:** recordings are trimmed to about 6 ticks on three days (day 1, a mid-tournament weekday, day 15) before committing to `testdata/live/`. The full set stays outside the repository.

---

## Appendix F: Rating calibration grid

Starting ratings for the 1960 warm-up period (also the fallback when a rank band has too little history):

| Band | Y | O | S | K | M1–5 | M6–10 | M11–17 | J1–7 | J8–14 | Ms |
|---|---|---|---|---|---|---|---|---|---|---|
| Rating | 1750 | 1700 | 1650 | 1625 | 1575 | 1550 | 1525 | 1475 | 1450 | 1425 |

Search ranges:

| Parameter | Values |
|---|---|
| `K` | 8, 10, 12, 16, 20, 24, 32 |
| `K_new` | 24, 32, 40, 48 |
| bouts counted as "new" | 15, 30, 45 |
| `r` (pull after a missed tournament) | 0, 0.1, 0.2, 0.3, 0.5 |
| warm-up table scale (spread around 1500) | 0.5, 1.0, 1.5 |

That is 1,260 combinations × about 195,000 bouts, about 4 seconds on 10 cores. `K` 8 and 10 were added after the first run chose 12, the lowest value then, so that the choice lies inside the grid. The best combination by log loss on the training years wins; ties go to the smaller `K`. The report in `docs/rating.md` lists the top 5 combinations with all metrics and both baselines.
