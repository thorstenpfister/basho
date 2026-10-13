# basho — Specification

> Status: **Draft v0.7** (2026-10-13) — for iteration. Nothing here is implemented yet.
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
- **Bout view ("VS screen"):** name, rank, stable, hometown, age, record going into the bout, last 5 results, height and weight bars, kinboshi stars, yusho count, head-to-head record, technique profile (fighter type, style, signature move). The result can be revealed on the screen.
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
- A wrestler encyclopedia or search screen.

### 1.3 Decisions so far

| # | Decision | Choice |
|---|---|---|
| D1 | Language and TUI stack | Go, Charm v2 suite (Bubble Tea, Lip Gloss, Bubbles, Harmonica, Fang) |
| D2 | VS screen layout | "Tale of the tape": mirrored fighters around a centre column of stat labels |
| D3 | Divisions | Makuuchi and Juryo, toggled in the UI |
| D4 | Spoilers | Results shown by default; spoiler-free mode can be configured |
| D5 | Names | Romaji shown prominently, kanji alongside |
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
| D23 | Portraits | Optional pixel-art portraits from Wikimedia Commons, built when the snapshot is built; **after 1.0** (§4.4, §8.9, Phase 14) |
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
| `GET /measurements?bashoId={id}` | Height and weight of **every** wrestler **at that tournament**, in one request | Sparse before about 2000; fall back to the profile |
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
| `GET /banzuke.json` | Current ranking with kanji `name`, `yomi` (reading), `id` (= JSA ID), `results[15]` (`win`, `loss`, `draw` or `null`) | **Doesn't distinguish fusen wins/losses or absences**; they appear as `null` or win/loss. Prefer sumo-api.com for those details once it has caught up. |
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
    - measurements for those wrestlers
  - `rikishi.json.zst`: every wrestler who appeared in Makuuchi or Juryo since 1960, with IDs (`id`, `nskId`, `sumodbId`), names, heya, birth date, hometown, debut and retired flag.
- **Format:** compact JSON compressed with zstd (`github.com/klauspost/compress/zstd`). Each tournament file is written once and never changes, so an update only **adds** files to git. Only `rikishi.json.zst` and the manifest are rewritten.
- **Size estimate:** about 400 tournaments × about 525 bouts ≈ 210,000 bouts, which should be 4–6 MB compressed (to be measured, V8). If it exceeds 15 MB, it ships as a separate release asset instead of being embedded.
- **Building it:**
  - **First full build:** about 400 tournaments × about 34 requests (1 basho, 2 banzuke, 30 torikumi, 1 measurements) ≈ 13,600 requests, about 4 hours at 1 request per second. This runs once, not on a tournament day (D24).
  - **Updates:** the tool reads the manifest and only fetches tournaments that are finished (rule in §7.2) and not yet included. That is about 34 requests per new tournament, plus 1 for the active-wrestler index.
  - **Resumable:** requests go through the normal `httpx` client (§7.1) and a work cache (`.cache/basho-data/`, ignored by git), so an interrupted run continues without fetching anything twice.
  - **Verification:** `basho-data snapshot verify` checks that each wrestler's bouts match their banzuke record and totals, that every bout appears for both wrestlers, that the schema is valid and that the checksums match.
- **Update flow:** `make snapshot` (update and verify), commit `data: snapshot through 202611`, then tag a patch release. There is no fixed schedule: the app works with a snapshot of any age, because it fetches whatever came later (§5.8).
- **Licence:** sumo-api.com publishes no terms beyond asking for responsible use. We stay within that through the limits above and credit sumo-api.com in the README, in `basho credits` and in the manifest (D24).

### 4.4 Wikimedia Commons and Wikidata (portraits, build time only)

Only `basho-data` uses these, for the optional portraits (§8.9). The app itself never contacts Wikimedia.

- **Join:** Wikidata property P3385 "Japan Sumo Association ID" equals sumo-api's `nskId`. Property P18 points to the image on Commons.
- **Coverage (checked 2026-10-13):**
  - 538 Wikidata items have a JSA ID, 293 of them with an image.
  - 35 of the 42 current Makuuchi wrestlers have an image.
- **Licences:** the samples checked are CC BY-SA 3.0 or 4.0. Only licences on an allow-list are accepted: CC0, public domain, CC BY 2.0–4.0, CC BY-SA 2.0–4.0.
- **Being polite:**
  - Follow Wikimedia's User-Agent policy (tool name and contact).
  - At most 1 request per second.
  - Fetch thumbnails only (`Special:FilePath/<file>?width=480`), never full-resolution originals.

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
- `tui` and `cli` only talk to `service`.
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

type Outcome int               // Win, Loss, FusenWin, FusenLoss, Absent, Draw, Pending, Unknown

type DayResult struct {
    Day        int
    Outcome    Outcome
    OpponentID RikishiID
    Kimarite   Kimarite        // canonical romaji key, e.g. "yorikiri"
}

type BanzukeEntry struct {
    RikishiID RikishiID
    Shikona   Name             // as of this tournament
    Rank      Rank
    Division  Division
    Results   [15]DayResult    // index = day-1
}

type BoutSide struct { RikishiID RikishiID; Shikona Name; Rank Rank }
type Bout struct {
    BashoID   BashoID
    Division  Division
    Day       int              // 1..15
    MatchNo   int              // 1-based display order
    East, West BoutSide
    Winner    *Side            // nil = not yet decided
    Kimarite  Kimarite
    IsPlayoff bool
}

type Name struct { En, Jp, Kana string }   // Jp cleaned (see §5.6)

type Rikishi struct {
    ID RikishiID; NSKID int; SumoDBID int
    Name Name; Heya string; Shusshin string
    BirthDate Date; Debut BashoID
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
    Measurements(ctx, BashoID) (map[RikishiID]Measurement, error)
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
| Profiles, measurements | sumo-api (no fallback; fields left empty) | same |
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
- An in-memory **bout index** (by wrestler and by tournament) is built from the snapshot the first time it is needed (estimated under 200 ms, measured in Phase 4) and extended with later tournaments and live results.
- Provenance for snapshot data reads `snapshot (2026-11-25)`, the snapshot's build date.

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
| `goreleaser` | Release builds (Phase 13) | Homebrew; `make tools` reports if missing |

**After 1.0 (portraits):** `github.com/esimov/pigo` (face detection), `golang.org/x/image/draw` (scaling), `github.com/charmbracelet/x/mosaic` (half-block rendering, or our own renderer), and `vhs` (Homebrew) for the README GIF.

**Charm v2 API facts and import-path rules:** `docs/charm-v2-notes.md`. Versions: `go.mod`.

**Deliberately not used:** Viper (TOML plus our own precedence rules is enough), testify (standard `testing` plus go-cmp), an HTTP client library, a dependency injection framework, a database (plain files and embedded data cover everything).

---

## 6. Derived data (`derive/`)

All inputs come from the bout index (§5.8). Every computation takes an **as-of cutoff** `AsOf{Day int; MatchNo int; Inclusive bool}`. This one mechanism handles three things:
- the correct record and stats going into a bout
- standings after day N
- spoiler-free mode (§9)

### 6.1 Record going into a bout

Record = wins and losses from `Results[0 : day-1]`, plus same-day bouts before this one (only playoffs can cause a second bout on the same day).
- Fusen wins/losses count as wins/losses.
- Absences are counted separately and shown as `W-L-A` only when A > 0, for example `5-7-3`.

### 6.2 Last 5 results

The wrestler's last 5 bouts **before** the cutoff, taken from the cached career matches, **crossing tournament boundaries** (on day 2 this shows 1 result from this tournament and 4 from the previous one, separated by a thin divider `│`). Absences are skipped. Fusen is shown as □/■.

Before the matches have loaded, use only this tournament's banzuke record and show an "…" placeholder for the rest.

### 6.3 Kinboshi

Rule: count the wrestler's matches where
- the wrestler's rank title at the time was `Maegashira`,
- the opponent's rank title was `Yokozuna`,
- the wrestler won,
- `kimarite ≠ fusen`,
- the match was in the Makuuchi division,
- and the match was before the cutoff.

The result is cached per wrestler as `{count, lastBashoCovered}` and topped up when new matches arrive.

Display:
- `★` per kinboshi. Five or more becomes `★×12`.
- Kinboshi from **this tournament** are drawn in a brighter gold.
- Shown for every wrestler, not only current Maegashira, because a san'yaku wrestler can have kinboshi from earlier tournaments.

Also stored (shown in the day recap, §8.5): `ThisBashoScalps` = wins against Yokozuna/Ozeki in this tournament.

### 6.4 Head-to-head record

Computed from one wrestler's cached career matches, filtered to the opponent's ID, before the cutoff. It contains:
- wins for each side
- each side's top 2 winning techniques
- the last 3 meetings (tournament, day, winner, technique)

The API's `/matches/{b}` endpoint is not used for this, because it can't apply a cutoff.

### 6.5 Standings after day N

Inputs: banzuke entries, cut off at day N.
- **Sort order:** wins descending, losses ascending, then banzuke rank. Grouped into bands: `Leader(s)`, `−1`, `−2`, then everyone else collapsed (expandable).
- **Contention:** a wrestler is still in contention if their wins + days remaining ≥ the leader's wins. Show "✓ yusho" once the leader's wins are more than anyone else's maximum possible wins. A playoff is possible if wrestlers end up tied.
- **Markers:**
  - 8+ wins: kachi-koshi, green, `KK`
  - 8+ losses: make-koshi, red, `MK`
  - kachi-koshi or make-koshi reached **on day N**: bold "new" marker
  - out for the rest of the tournament: `休` (absent)
- **Hoshitori grid:** one row per wrestler in banzuke order, 15 result columns, with day N's column highlighted.

### 6.6 Career statistics as of a date

- **Height and weight:** from `Measurements(bashoID)`, so they are as of that tournament. If missing, fall back to the profile and mark "(today)".
- **Age:** birth date to tournament start date.
- **Yusho and sansho:** counted exactly as of the tournament, from each tournament's yusho and sansho lists in the snapshot plus later tournaments (§5.8). The current tournament's results only count once they fall before the cutoff, so nothing is spoiled.
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
- Default: the **last 6 tournaments** before the as-of cutoff (§6), including earlier days of the current tournament.
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

  The thresholds are constants for now; tune them in Phase 3 against a sample of well-known wrestlers.
- **Matchup line:** A's main winning category against the share of B's losses in that category, and the same the other way round. Show the direction with the larger product.

**Spoilers:** the as-of cutoff applies, so the profile only uses bouts before the bout being viewed (and before the reveal horizon in spoiler-free mode).

**CLI:** `basho bout` prints both profiles under the tale of the tape. `--json` includes them as `moveset` objects.

### 6.8 Rating and win chance

There is no public sumo rating API. Ozeki Analytics published an Elo analysis covering 1958 to today as blog posts (no data feed), which is useful as a plausibility check. We compute our own rating.

**Model: Elo**, centred on 1500, processed bout by bout in date order over all sekitori bouts.

- Win chance for A against B: `1 / (1 + 10^((R_B − R_A) / 400))`.
- Fusen bouts and draws are skipped. Playoff bouts count.
- **Starting rating:** lower-division bouts aren't available. So a wrestler's first sekitori bout starts at the average rating of wrestlers in the same rank band over the previous 6 tournaments.
  - Bands: each san'yaku title, M1–5, M6–10, M11–17, J1–7, J8–14.
  - During the 1960 warm-up period, when not enough history exists yet, a fixed table by rank is used.
- **K factor:** a higher value (`K_new`) for a wrestler's first 30 sekitori bouts, then `K`.
- **Return after absence:** for each tournament missed completely (fully absent, or ranked below Juryo), the rating is pulled a fraction `r` towards the rank-band average at the return.
- The parameters `K_new`, `K` and `r` are stored in `data/rating/params.json` (embedded) and come from calibration.

**Calibration** (`basho-data rating calibrate`):

- Grid search over the parameters, training on 1960–2014 and evaluating on 2015 to the newest tournament in the snapshot.
- Metrics: log loss, Brier score, accuracy, and a calibration table (predicted vs actual in 10% buckets).
- Baselines: 50/50, and "higher rank wins" with a probability fitted by rank difference.
- **The rating is only shown if it beats the rank baseline on log loss** (V9). Otherwise the feature stays hidden and the model is revisited (for example Glicko-2, or a blend with rank).

**At runtime:**

- Ratings are recomputed from the bout index the first time they are needed (about 210,000 updates, a few milliseconds) and continue through later tournaments and live results.
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
- Each entry stores: body (gzipped), `FetchedAt`, `ETag`, `Last-Modified`, `SourceUpdatedAt`, `DataClass`, schema version.
- Freshness is decided by data class:

  | Data class | Fresh for (TTL) | Notes |
  |---|---|---|
  | Finished tournament: metadata, banzuke, bout lists, measurements | **forever** | Finished = end date + 2 days has passed **and** a yusho winner exists. The 2-day margin allows for late corrections. |
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
| Open a tournament that finished after the snapshot, day 1, one division | 4 requests: basho, banzuke, torikumi, measurements | 0 |
| Move to another day (not in the snapshot) | 1 | 0 |
| Toggle division (not in the snapshot) | 2 (banzuke and torikumi) | 0 |
| Open a VS screen | **0** (all statistics come from the bout index); ≤ 4 without a snapshot | 0 |
| Wrestler index | 1 request (active wrestlers) in the background, only when the snapshot's index is older than 7 days | 0 |
| Prefetch | **none by default**. Optional (`prefetch.adjacent_days = true`): the next day only, and only when idle for more than 2 s. | |
| Live mode, per poll | 1 conditional GET to osada (usually 304 or a small body) | |
| Live mode, per tournament day | ≈ 1 request every 3 min over ≈ 4.5 h ≈ **≤ 90 requests**, mostly 304 | |

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
  | 60–89 (compact) | Single column, short ranks (`M17e`), kanji hidden on the day view, VS screen without bars |
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
- Yusho text is filled in only from the cache (finished tournaments already viewed). Otherwise the row is left empty. Browsing the list never triggers requests.
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
  - The last bout of the day gets a `結び` tag. Playoff bouts are grouped below a `── Kettei-sen ──` divider.
- **Live updates:** newly decided bouts briefly show a `NEW` badge until the next poll or 60 s. The selection never moves when the list updates.
- **Wide layout:** right-hand panel with the yusho race after the selected day (top two bands).
- **Kyujo footer:** wrestlers absent on that day (osada `absentees`, or sumo-api records with `absent`).
- **Day recap (`r`)**, a short "between rounds" summary of the day:
  - leaders after the day
  - who reached kachi-koshi or make-koshi today
  - kinboshi earned today
  - upsets (the winner's win chance before the bout was below 35%, §6.8)
  - technique of the day (the rarest technique used)

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
   ██████████████ 190cm       HEIGHT        188cm █████████████
   ██████████████ 188kg       WEIGHT        150kg ██████████
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

(The values in the mockup are illustrative. In the STYLE bars, each segment has its own colour; the shading only stands in for colour here. See §6.7.)

- Layout: mirrored columns around a centre column of labels. The left column is right-aligned towards the centre and the right column left-aligned (both "face" the middle).
- **Bars:** height and weight are scaled to the division's minimum–maximum for that tournament, so differences are visible. Each pair's bars point outwards from the labels.
- **Result panel:**
  - Normal mode: visible when the bout is decided.
  - Spoiler-free mode: shows `[space] reveal` instead.
  - Undecided bout: `Scheduled — Day 13`, or in live mode `Waiting for result…`.
  - Content: winner, technique in romaji and kanji with its English gloss on a third line, records after the bout. A kinboshi bout gets a gold `★ KINBOSHI` stamp; a fusen bout gets `FUSEN (default)`.
- **Head-to-head history (`v`):** expands the last 3 to 10 meetings (tournament, day, ranks, winner, technique). In the wide layout it is a side column and always visible.
- **RATING and WIN CHANCE** (§6.8): ratings before the bout. The win-chance bar is drawn from the centre outwards on both sides, so the favourite's bar is longer. Hidden if the rating didn't pass calibration (V9).
- **Fighter type and STYLE:**
  - The fighter type (§6.7) is shown dimmed under the kanji name, like a character class.
  - The STYLE row is a 20-cell stacked bar per wrestler, coloured by category, in a fixed order (PUSH, BELT, THROW, PULL, OTHER). On the left side the order is reversed so that PUSH sits next to the centre label on both sides.
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

A full key list grouped by screen (Bubbles `help` in full mode), plus a legend for the glyphs and abbreviations and the data-source notes (osada AI-use policy, sumo-api is run by a volunteer).

### 8.8 Keybindings

| Key | Context | Action |
|---|---|---|
| `←`/`h`, `→`/`l` | day view, standings | previous/next day |
| `1`–`9`, `0` (=10), `!` `@` `#` `$` `%` (=11–15) | day view | jump to day; the numbers 11–15 are also reachable as a `g` prefix: `g1` `3` |
| `↑`/`k`, `↓`/`j` | lists | move |
| `enter` | day view | open VS screen |
| `[` / `]` | VS screen | previous/next bout |
| `space` | VS screen | reveal result (spoiler-free mode) |
| `v` | VS screen | head-to-head history |
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
| `?` | anywhere | help |
| `q`, `ctrl+c` | anywhere | quit (from the VS screen `q` goes back one level; `ctrl+c` always quits) |

*Note:* `h`/`l` always mean left/right (previous/next) on every screen; head-to-head uses `v` (D16).

---

### 8.9 Portraits (optional, after 1.0)

Pixel-art portraits beside the tale of the tape, facing each other like a fighting-game character select screen:

```
  ▗▄▄▄▄▖                                                        ▗▄▄▄▄▖
 ▐▓▓▓▓▓▓▌  ONOSATO                VS                HOSHORYU   ▐▓▓▓▓▓▓▌
 ▐▓▀██▀▓▌  大の里                                       豊昇龍   ▐▓▀██▀▓▌
 ▐▓▓▄▄▓▓▌  Pusher-thruster                       Belt fighter   ▐▓▓▄▄▓▓▌
  ▜████▛   ──────────────────────────────────────────────────    ▜████▛
 (32×32 px = 32 columns × 16 rows in the real UI; this is only a stand-in)
```

**Build pipeline** (`basho-data portraits update`, runs together with the snapshot):

1. Query Wikidata for wrestlers in the snapshot's index with P3385 and P18 (§4.4). Fetch the licence, author and source URL from the Commons API and drop anything not on the allow-list.
2. Fetch a 480 px thumbnail.
3. Crop to head and shoulders. Face detection uses `github.com/esimov/pigo` (pure Go) and crops a square about 2.2× the face box. Manual overrides in `data/portraits/overrides.toml` can set the crop box, exclude an image or pick a different file.
4. Downscale with area averaging to 32×32 (wide) and 20×20 (normal), then reduce colours to the theme palette (default: a 32-colour "arcade" palette) with optional ordered dithering.
5. Store as PNG (about 1 KB each, about 300 KB in total) in `data/portraits/`, together with `credits.json` and a generated `CREDITS.md` (file, author, licence, source URL, and the changes made: "cropped, downscaled, colour-reduced").

**Licensing:** the derived images are adaptations, so they keep the source licence (mostly CC BY-SA). `data/portraits/LICENSE` says so. Attribution is shown by `basho credits`, on a credits page in the help overlay, and in the README.

**Rendering:**

- Half-block characters (`▀`): the foreground colour is the upper pixel, the background colour the lower one. This is pure text, so it fits into Bubble Tea and Lip Gloss layouts and works in any truecolor terminal (Ghostty included). `github.com/charmbracelet/x/mosaic` (experimental) or a small renderer of our own.
- The West portrait is mirrored horizontally so the fighters face each other.
- On 256-colour terminals, colours map to the nearest palette entry. Portraits are hidden with `NO_COLOR`, `--ascii`, in compact layout and with `ui.portraits = false`.
- Size: 32 px portraits at ≥ 140 columns, 20 px at 110–139, none below that.
- Below each portrait a dim caption gives the photo year ("photo 2014"), because some photos are old.
- **No photo:** a procedurally generated silhouette sprite, shaped by height and weight, with a mawashi colour derived from the wrestler ID. It is clearly not a likeness.
- Full-resolution images via terminal graphics protocols (Kitty, iTerm2, Sixel) are not used: they conflict with Bubble Tea's renderer (§15).

---

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
basho watch [--division]                # live line stream (phase 11)
basho ratings [-t] [--top N]            # top-rated wrestlers as of a tournament (phase 5)
basho credits                           # data sources and portrait attributions
basho version
```

Global flags:

| Flag | Meaning |
|---|---|
| `--json` | Machine-readable output |
| `--source auto\|sumoapi\|osada` | Force a data source |
| `--no-color`, `--ascii` | Plain output |
| `--config PATH` | Alternative config file |
| `--debug` | Debug log |
| `--offline` | Snapshot and cache only, never touch the network |
| `--no-snapshot` | Hidden, for development: ignore the embedded snapshot |

Output rules:

- **When the output is a terminal (TTY):** Lip Gloss tables in the same style as the TUI.
- **When piped:** no colour, same layout.
- **`--json`:** a stable envelope:

  ```json
  {
    "schemaVersion": 1,
    "kind": "day",
    "provenance": {"source":"sumo-api","fetchedAt":"…","sourceUpdatedAt":null,"stale":false},
    "data": { … domain object, camelCase … }
  }
  ```

  Enums use stable lowercase strings (`"win"`, `"fusen_loss"`, `"maegashira"`). The schema is documented in `docs/json.md`, and golden files lock it down.

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
- Precedence: flags > environment (`BASHO_*`, for example `BASHO_SOURCE=osada`) > file > defaults.
- `basho config path|show` prints the effective config (phase 12).

```toml
[ui]
division      = "makuuchi"    # start division
names         = "both"        # both | en | jp   (D5; hidden option)
ascii         = false
animations    = true
mouse         = true
theme         = "auto"        # auto | dark | light
win_chance    = true          # RATING and WIN CHANCE rows on the VS screen (D26)
win_chance_column = false     # day view column (toggle: p)
portraits     = true          # only once portraits ship (phase 14)

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

[cache]
dir           = ""            # default XDG cache
max_size_mb   = 200

[prefetch]
adjacent_days = false
```

---

## 13. Testing strategy

- **Fixtures** (`testdata/`), recorded once with a `make fixtures` script that calls slowly, 1 request every 2 s:
  - sumo-api `202609`, finished (Makuuchi and Juryo banzuke, days 1, 13 and 15, measurements, matches for about 6 wrestlers including a kinboshi holder)
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
  - A three-tournament fixture snapshot (202607, 202609, 196001) is built by `basho-data` from the recorded fixtures, then loaded by the app.
  - The answers must equal those from the live adapter path for the same data.
  - `verify` must catch deliberately corrupted files.
- **Rating tests:** Elo update maths and the starting-rating and absence rules on a tiny hand-made bout set. A regression test pins the calibration metrics of the committed `params.json`, so a model change has to show it doesn't get worse.
- **Portrait pipeline tests:** crop, downscale and quantize are deterministic on a fixture image (golden PNG). Licence allow-list, credits generation and the half-block renderer (golden text) are covered too.
- **Adapter contract tests:** one suite runs against both adapters with their fixtures and checks that the domain output is equivalent for the same tournament and day.
- **httpx tests** (`httptest.Server`): rate limit timing (inside `testing/synctest`, so `internal/clock` stays a plain `Now()` clock), retry and jitter limits, 429 + `Retry-After`, circuit breaker state changes, ETag/304, singleflight.
- **Cache tests:** TTL per data class, stale-while-revalidate, LRU eviction, schema version change.
- **Request-budget tests:** a counting `RoundTripper` checks the numbers in §7.3.
- **Live scheduler tests:** fake clock covering the window, jitter range, doubling interval, stop conditions and day 15 extension.
- **TUI tests:** `teatest` with a fixed 120×40 and 80×24 size and a fake clock. Golden snapshots of each screen; key flows (picker → day → VS → reveal → back); resizing; spoiler-free flow.
- **CLI tests:** golden text output and JSON schema golden files.
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
| `test-live` | `go test -tags live ./...`: real API smoke tests, manual only | 1 |
| `golden` | Re-runs tests with `-update` to rewrite golden files (review the diff afterwards) | 6 |
| `cover` | Coverage profile and HTML report (`coverage.html`) | 0 |
| `fmt` | `go tool gofumpt -w .` | 0 |
| `fmt-check` | Fails if gofumpt would change anything | 0 |
| `lint` | `golangci-lint run` | 0 |
| `vuln` | `go tool govulncheck ./...` | 0 |
| `tidy` | `go mod tidy` and `go mod verify` | 0 |
| `check` | `fmt-check lint vuln test-race`; the full check before committing | 0 |
| `tools` | Checks that external tools exist (golangci-lint; goreleaser from Phase 13) and prints install hints | 0 |
| `fixtures` | `basho-data fixtures record`: polite recording of the test fixtures | 1 |
| `record` | `basho-data record $(ARGS)`: live recorder (Appendix E) | 1 |
| `snapshot` | `basho-data snapshot update` then `verify` | 3 |
| `snapshot-verify` | `basho-data snapshot verify` | 3 |
| `snapshot-stats` | `basho-data snapshot stats` | 3 |
| `calibrate` | `basho-data rating calibrate`; writes `data/rating/params.json` and `docs/rating.md` | 5 |
| `vhs` | Renders the README GIF from `docs/demo.tape` | 12 |
| `release-snapshot` | `goreleaser release --snapshot --clean` into `dist/` | 13 |
| `portraits` | `basho-data portraits update` | 14 |
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

- A wrestler screen: career graph of rank and rating over time (sparkline), technique usage, all tournaments.
- Favourites, filters, notifications (decided against for v1).
- `basho cache info|clear` (not selected for v1; `--offline` and the automatic cache cover the basics).
- "Streak" markers (🔥 5 wins in a row).
- Theme packs (for example "arcade" with a block-font `VS` and a matching portrait palette).
- Signing and notarization (D25).
- A better rating model (Glicko-2, or a blend with rank and head-to-head) if plain Elo shows weaknesses.
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
- ~~Q12 Portraits~~ → after 1.0 (D23)
- ~~Q13 Win chance visibility~~ → shown by default on the VS screen (D26)

**To check (V = verification):**

- **V1** How quickly sumo-api results appear during the live window. Measure during Kyushu 2026 (2026-11-08 to 22) by recording `resultUpdatedAt` against the first appearance of each result in sumo-api.
- **V2** Whether sumo-api includes playoff bouts in day 15's bout list, and how they are marked (`matchNo` above the regular count?).
- **V3** Old-era outcomes (draws `hikiwake`, `azukari`) and how sumo-api encodes them.
- **V4** Maximum `limit` for `/rikishis` (pagination).
- **V5** Whether sumo-api supports `ETag`/`Last-Modified` (none seen so far; it uses nginx).
- **V6** How osada represents a playoff once it is decided (`kimarite: 未定` was seen on playoff entries after the tournament ended).
- **V7** Side inconsistencies: a wrestler listed on the West side of sumo-api's banzuke with an "East" rank string (Aonishiki "Ozeki 2 East", seen in a match record). Decide which one is authoritative (the rank string).
- **V8** Actual snapshot size (compressed) and time to build the bout index at startup.
- ~~**V10**~~ **Answered (Phase 1):** `rankValue` = title × 100 + number (Yokozuna 1 → 101, Juryo 3 → 603); the side is not encoded, East and West share the value. Old-era haridashi wrestlers appear as ordinary numbered ranks (196001 has "Yokozuna 2 East"), so the English format is the same throughout. Side note for §6.8: Juryo had 18 ranks per side in 1960, not 14, so the rank bands must not assume J14 is the lowest.
- **V11** Whether our kimarite keys (Appendix B) exactly match sumo-api's spellings. Compare against the `/kimarite` endpoint, which needs a `sortField` parameter (`count`, `kimarite` or `lastUsage`).
- **V9** Whether Elo beats the "higher rank wins" baseline on log loss for 2015 onwards. This decides whether the win chance is shown at all.

---

## 17. Implementation plan

Each phase ends with something usable and has acceptance criteria (AC). Phases 1–6 build and prove the data foundation through the CLI before the TUI starts.

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
4. `source/sumoapi` DTOs for basho, banzuke, torikumi, rikishis (paged), rikishi, matches, stats and measurements. The "zero object" for unknown tournaments becomes `ErrNotYetAvailable`.
5. Mapping from DTOs to the domain, with an explicit strategy for unknown values.
6. A slow, polite fixture recorder (`make fixtures`) and the fixtures from §13.
7. Unit tests: rank parsing, calendar, tournament argument parsing, mapping for 202609, 196001 and 202607.
8. Answer V2, V3, V4, V5, V7 and V10 from the fixtures and record the answers in this spec.
9. Live recorder `basho-data record` (Appendix E), on a minimal `httpx` (User-Agent, timeouts, conditional requests, minimum interval per host) that Phase 2 extends. Runs on an always-on machine via launchd; see `docs/runbooks/kyushu-recorder.md`.

**AC:** a test can load the 202609 Makuuchi banzuke and the day 13 bout list from fixtures into domain objects, with every outcome type represented; V2–V5 and V7 are documented.

### Phase 2: Polite HTTP and cache

1. `httpx`: client with User-Agent, gzip and timeouts; per-host rate limit; concurrency limit; retries with jitter; 429 and `Retry-After`; circuit breaker; ETag/Last-Modified; singleflight; debug log.
2. `cache`:
   - file store with entry metadata and the TTL table from §7.2
   - stale-while-revalidate API (`Get` returns the value and its freshness, plus a background refresh hook)
   - LRU size cap, schema versioning, `--offline` support
3. Data-class classification, including detecting finished tournaments (end date + 2 days and a yusho winner present).
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
7. Run the full build once (about 4 hours, not on a tournament day), measure V8, commit `data/snapshot/`.

**AC:** with network access blocked, any tournament from 196001 to the snapshot's newest one loads in under 100 ms; `verify` passes on the committed snapshot; a second `snapshot update` without new tournaments makes no requests apart from the manifest check.

### Phase 4: Derived data engine

1. The `AsOf` cutoff type and helpers.
2. Record going into a bout and after it, including absences and fusen.
3. Last 5 across tournament boundaries from the bout index, padded for newly promoted wrestlers.
4. Kinboshi count and `ThisBashoScalps`.
5. Head-to-head summary (wins, top techniques, last N meetings).
6. Standings after day N: sorting, bands, contention calculation, yusho decided, kachi-koshi/make-koshi markers including "reached today", hoshitori grid model.
7. Career statistics as of a date: measurements per tournament with fallback, age, exact yusho/sansho as of the tournament, "now: X" when the shikona has changed.
8. Day recap: leaders, kachi-koshi/make-koshi reached today, kinboshi today, upsets (using the rating once Phase 5 is done; rank gap until then), rarest technique.
9. Technique profile (§6.7): category shares, sample window with widening, signature move, fighter type, matchup line. Tune the fighter-type thresholds against about 10 well-known wrestlers and record the final values in this spec.
10. Table-driven tests for each, including edge cases: absence then return, fusen against a yokozuna, playoff, Juryo wrestler in Makuuchi, fewer than 10 sekitori wins.

**AC:** standings for 202609 after day 15 match the official final records; kinboshi counts for 3 chosen wrestlers match published totals (checked by hand once against Wikipedia); bout-index build time is recorded (V8).

### Phase 5: Rating and win chance

1. `rating` package: Elo update, rank-band starting ratings with the fixed table for the warm-up period, `K_new`/`K`, adjustment after absences, as-of queries, continuing through live results.
2. `basho-data rating calibrate`: grid search, train/test split, log loss, Brier score, accuracy, calibration table, both baselines; writes `data/rating/params.json`.
3. **Decide V9** and record the metrics in this spec. If Elo doesn't beat the rank baseline, the win-chance UI stays switched off and a revised model goes on the list.
4. Plausibility check: the top-rated wrestlers of all time compared with published Elo lists (Ozeki Analytics) and common sense.
5. Service output: rating, change since the tournament start, win chance; `basho ratings` command.
6. Tests from §13 (maths, rules, metric regression).

**AC:** the calibration report is committed under `docs/rating.md`; `params.json` is embedded; win chances for a sample of bouts are plausible and identical between two runs (deterministic).

### Phase 6: Service layer and non-interactive CLI

1. `service` façade: `CurrentBasho`, `Basho`, `Day(basho, division, day)`, `Bout(basho, division, day, matchNo)` (the full tale-of-the-tape model, including moveset and rating), `Standings(basho, division, day)`, `Recap`, `Ratings`. Each returns `Result[T]` with provenance.
2. Background refresh of the active-wrestler index when the snapshot's index is older than 7 days, plus the `idmap` skeleton (filled in Phase 9).
3. Config loading (file, environment, flags, defaults) and validation, including minimum intervals and maximum rates.
4. Commands: `list`, `info`, `day`, `bout`, `standings` (`--grid`), `ratings`, `credits`, with text renderers (Lip Gloss tables in a terminal, plain when piped).
5. The `--json` envelope, `docs/json.md` and exit codes.
6. Golden tests for text and JSON.

**AC:** `basho day 13 -t 202609`, `basho bout 13 19 -t aki2026` and `basho standings --day 15 -t 2026-09 --grid` give correct, readable output with no network access (snapshot), and their `--json` versions match the golden files.

### Phase 7: TUI shell, picker and day view

1. Theme package: adaptive palette, styles, Unicode and ASCII glyph sets, `NO_COLOR`.
2. Root model: screen stack, global keys (§8.8), size breakpoints, "too small" screen, status bar with provenance and short-lived messages, help overlay.
3. Basho picker built from the calendar; yusho text from the snapshot (instant for history) and the cache; `/` filter; cancelled rows.
4. Day view:
   - day bar with status marks
   - bout table (winner bold, loser dim, records, techniques, `結び`, playoff divider)
   - Kyujo footer
   - `d` division toggle, number and `g` jumps
   - `p` win-chance column
5. Loading, empty and error states; stale-while-revalidate redraws; `R` refresh with cooldown.
6. Wide layout side panel (top bands of the yusho race).
7. `teatest` golden snapshots (80×24, 120×40) and navigation flow tests.

**AC:** starting `basho` lands on the latest tournament day; every day since 1960 can be browsed in both divisions without network access, except tournaments newer than the snapshot; resizing never breaks the layout.

### Phase 8: VS screen

1. Components:
   - stat row (mirrored values around a centre label)
   - bar, scaled to the division's range for that tournament
   - result glyphs, kinboshi stars
   - win-chance bar
2. Layout of the tale of the tape (§8.5), compact and wide variants, head-to-head line and expandable history (`v`).
3. Progressive loading for tournaments not in the snapshot: draw immediately from bout and banzuke data, fill in the rest with `…` placeholders.
4. Result panel: winner, technique (romaji, kanji, English), records after the bout, KINBOSHI and FUSEN stamps, undecided and waiting states.
5. Technique rows: fighter type under the name, mirrored STYLE stacked bar, SIGNATURE row, "not enough data" state. `m` opens the moveset overlay, `w` cycles its window, and it shows the matchup line.
6. RATING and WIN CHANCE rows, hidden if V9 failed.
7. `[`/`]` navigation including crossing day boundaries.
8. Harmonica slide-in and reveal flash; switched off by config, compact layout, or when the output isn't a terminal.
9. Golden snapshots for: a regular bout, a kinboshi bout, a fusen bout, an undecided bout, an old-era bout without kanji or measurements, a newly promoted wrestler.

**AC:** every Makuuchi bout of 202609 renders without layout problems at 90 and 130 columns; opening a VS screen for a snapshot tournament makes no requests.

### Phase 9: Standings, recap and osada adapter

1. Standings screen:
   - tab A: race, bands, collapsed tail, contention and yusho markers
   - tab B: hoshitori grid with the day column highlighted
   - `←`/`→` move the cutoff; wrestler popup
2. Day recap overlay (`r`).
3. `source/osada`:
   - DTOs for `torikumi.json` and `banzuke.json`
   - Japanese rank parser (kanji numerals, `枚目`, `張出`)
   - Japanese technique names → canonical keys (via the kimarite table); `未定` → pending
   - `isPlayoff`, absentees, status and timestamps
   - JSA ID taken from the profile URL
4. `idmap`: JSA ID ↔ sumo-api ID from the snapshot index plus the active index; refresh on an unknown ID (at most once an hour); kana→romaji fallback marked with `≈`.
5. `resolver`: the policy table (§5.4), "behind" detection, merge rules, per-bout source tracking, `--source` override, stale cache as last resort.
6. Contract tests: both adapters produce equivalent domain output for the current-tournament fixtures.

**AC:**
- With sumo-api disabled (in a test), the current tournament's day view and standings still render from osada, with English names via the ID mapping.
- With osada disabled, everything falls back to sumo-api.
- The status bar always names the source that answered.

### Phase 10: Live mode

1. `live.Scheduler`, pure and testable with a fake clock: windows from config, day 15 extension, jitter, doubling when nothing changes, stop conditions, final check, error backoff.
2. Wire it into the TUI:
   - `liveTickMsg` / `liveSnapshotMsg`, merged into the service cache, bout index and ratings
   - `NEW` badges, selection kept stable
   - status bar states (§10.3), `L` toggle, `live.auto`
3. Conditional GET for osada (the 304 path), with the sumo-api fallback each tick.
4. Use the Kyushu 2026 recordings and the V1 result to adjust the default interval and source priorities.
5. Scheduler and request-budget tests (≤ 90 requests per day, mostly 304).

**AC:** during a real tournament day the app shows new results within one poll interval of osada's update, stays within budget, and stops polling once the day is complete.

### Phase 11: Spoiler-free mode and `basho watch`

1. `spoiler` package: horizon per tournament and division, saved to the state file, reveal operations, a counter of hidden new results.
2. Apply the horizon everywhere:
   - derived cutoffs in the service, including ratings and win chance
   - `?` results in the day view
   - standings and recap locked to the horizon, with a banner
   - picker yusho hidden
3. VS screen `space` reveal flow; `ctrl+r` "reveal up to here" with confirmation; `S` session toggle.
4. CLI `--no-spoilers` and `--reveal-to`.
5. `basho watch` line stream (text and NDJSON), reusing the live scheduler.
6. Tests: no spoiler leaks through any view (snapshot tests with a horizon in the middle of a day), and the horizon persists between runs.

**AC:** in spoiler-free mode, a tester can step through day 13 bout by bout with `]` + `space`, and nothing on any screen reveals a later result, including ratings.

### Phase 12: Polish

1. Light/dark theme check, `--ascii` and `NO_COLOR` passes, mouse wheel support.
2. Help overlay content: legend, data source notes, credits page, an explanation of the rating, known limitations (historical heya; sekitori bouts only).
3. `basho config path|show`; friendlier first-run messages.
4. Performance: hoshitori grid renders in under 16 ms at 130×50; snapshot and bout index load fast enough for an instant start (target under 300 ms to first screen).
5. Accessibility: never rely on colour alone (glyphs and KK/MK text always shown).
6. VHS tape and GIF for the README.

**AC:** a full manual test pass (checklist in `docs/qa.md`) in Ghostty and one other terminal, in light and dark themes.

### Phase 13: v1.0 release (private)

1. `make install` with the ldflags; `basho version` shows the snapshot date from the manifest.
2. `.goreleaser.yaml`: targets, ldflags, archives with completions and man page, checksums. No Homebrew section yet.
3. Dry run with `make release-snapshot`.
4. Document the snapshot release flow (update → verify → commit → patch tag → install) in `docs/releasing.md`; CHANGELOG.
5. Tag `v1.0.0`.

**AC:** `make install` produces a `basho` whose `basho version` shows the tag and the snapshot date; zsh completions work; a snapshot-only patch release takes under 15 minutes of manual work.

**After 1.0:**

### Phase 14: Portraits (optional)

1. `basho-data portraits update`: Wikidata query (P3385 → P18), Commons metadata and licence allow-list, polite thumbnail download.
2. Face crop with `pigo` and the `overrides.toml` mechanism; downscale to 32 and 20 px; palette quantization with optional dithering.
3. `data/portraits/` with PNGs, `credits.json`, generated `CREDITS.md` and `LICENSE`; `basho credits` and the help credits page.
4. Half-block renderer with mirroring and the 256-colour fallback; procedural silhouette for wrestlers without a photo.
5. VS screen integration at the size breakpoints (§8.9), photo-year caption, `ui.portraits` toggle.
6. Manual review of all generated portraits for current Makuuchi and Juryo wrestlers, and fix crops via overrides.
7. Golden tests (§13).

**AC:** portraits render for every current Makuuchi wrestler with a Commons photo; every image is correctly credited; the VS screen still works at every size with portraits on and off.

---

### Phase 15: Public repository and Homebrew tap

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
- `GET /measurements?bashoId=202609` → array of `{"id":"202609-9144","bashoId":"202609","rikishiId":9144,"height":177,"weight":124}` for all divisions; `196001` → only 1 entry.
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

## Appendix B: Kimarite table (draft)

Canonical key = sumo-api spelling, lower case, no spaces or hyphens (V11). The category is our own grouping (§6.7, D17), not the JSA's. Phase 1 checks every row against the official JSA list and sumo-api's spellings.

| Key | Kanji | English | Category |
|---|---|---|---|
| oshidashi | 押し出し | frontal push out | PUSH |
| oshitaoshi | 押し倒し | frontal push down | PUSH |
| tsukidashi | 突き出し | frontal thrust out | PUSH |
| tsukitaoshi | 突き倒し | frontal thrust down | PUSH |
| okuridashi | 送り出し | rear push out | PUSH |
| okuritaoshi | 送り倒し | rear push down | PUSH |
| yorikiri | 寄り切り | frontal force out | BELT |
| yoritaoshi | 寄り倒し | frontal crush out | BELT |
| abisetaoshi | 浴びせ倒し | backward force down | BELT |
| kimedashi | 極め出し | arm barring force out | BELT |
| kimetaoshi | 極め倒し | arm barring force down | BELT |
| tsuridashi | 吊り出し | frontal lift out | BELT |
| tsuriotoshi | 吊り落とし | frontal lift body slam | BELT |
| okuritsuridashi | 送り吊り出し | rear lift out | BELT |
| okuritsuriotoshi | 送り吊り落とし | rear lift body slam | BELT |
| waridashi | 割り出し | upper-arm force out | BELT |
| sabaori | 鯖折り | forward force down | BELT |
| uwatenage | 上手投げ | overarm throw | THROW |
| shitatenage | 下手投げ | underarm throw | THROW |
| sukuinage | 掬い投げ | beltless arm throw | THROW |
| kotenage | 小手投げ | armlock throw | THROW |
| kubinage | 首投げ | headlock throw | THROW |
| koshinage | 腰投げ | hip throw | THROW |
| ipponzeoi | 一本背負い | one-armed shoulder throw | THROW |
| kakenage | 掛け投げ | hooking inner thigh throw | THROW |
| nichonage | 二丁投げ | body drop throw | THROW |
| tsukaminage | 掴み投げ | lifting overarm throw | THROW |
| uwatedashinage | 上手出し投げ | pulling overarm throw | THROW |
| shitatedashinage | 下手出し投げ | pulling underarm throw | THROW |
| yaguranage | 櫓投げ | inner thigh lifting throw | THROW |
| okurinage | 送り投げ | rear throw down | THROW |
| okurigake | 送り掛け | rear leg trip | THROW |
| ashitori | 足取り | leg pick | THROW |
| chongake | ちょん掛け | pulling heel hook | THROW |
| kawazugake | 河津掛け | hooking backward counter throw | THROW |
| kekaeshi | 蹴返し | minor inner foot sweep | THROW |
| ketaguri | 蹴手繰り | leg kick pull down | THROW |
| kirikaeshi | 切り返し | twisting backward knee trip | THROW |
| komatasukui | 小股掬い | over thigh scooping body drop | THROW |
| kozumatori | 小褄取り | ankle pick | THROW |
| mitokorozeme | 三所攻め | triple attack force out | THROW |
| nimaigeri | 二枚蹴り | ankle kicking twist down | THROW |
| omata | 大股 | thigh scooping body drop | THROW |
| sotogake | 外掛け | outside leg trip | THROW |
| sotokomata | 外小股 | over thigh scooping body drop | THROW |
| susoharai | 裾払い | rear foot sweep | THROW |
| susotori | 裾取り | ankle pick | THROW |
| tsumatori | 褄取り | rear toe pick | THROW |
| uchigake | 内掛け | inside leg trip | THROW |
| watashikomi | 渡し込み | thigh grabbing push down | THROW |
| izori | 居反り | backwards body drop | THROW |
| kakezori | 掛け反り | hooking backwards body drop | THROW |
| shumokuzori | 撞木反り | bell hammer backwards body drop | THROW |
| sototasukizori | 外襷反り | outer reverse backwards body drop | THROW |
| tasukizori | 襷反り | reverse backwards body drop | THROW |
| tsutaezori | 伝え反り | underarm forward body drop | THROW |
| amiuchi | 網打ち | fisherman's throw | THROW |
| gasshohineri | 合掌捻り | clasped-hand twist down | THROW |
| harimanage | 波離間投げ | backward belt throw | THROW |
| kainahineri | 腕捻り | two-handed arm twist down | THROW |
| kotehineri | 小手捻り | arm-locking twist down | THROW |
| kubihineri | 首捻り | head twisting throw | THROW |
| makiotoshi | 巻き落とし | twist down | THROW |
| osakate | 大逆手 | backward belt throw | THROW |
| sakatottari | 逆とったり | arm bar throw counter | THROW |
| shitatehineri | 下手捻り | twisting underarm throw | THROW |
| sotomuso | 外無双 | outer thigh propping twist down | THROW |
| tokkurinage | 徳利投げ | two-handed head twist down | THROW |
| tottari | とったり | arm bar throw | THROW |
| tsukiotoshi | 突き落とし | thrust down | THROW |
| uchimuso | 内無双 | inner thigh propping twist down | THROW |
| uwatehineri | 上手捻り | twisting overarm throw | THROW |
| zubuneri | ずぶねり | head pivot throw | THROW |
| utchari | うっちゃり | backward pivot throw | THROW |
| yobimodoshi | 呼び戻し | pulling body slam | THROW |
| hatakikomi | 叩き込み | slap down | PULL |
| hikiotoshi | 引き落とし | hand pull down | PULL |
| hikkake | 引っ掛け | arm grabbing force out | PULL |
| katasukashi | 肩透かし | under-shoulder swing down | PULL |
| okurihikiotoshi | 送り引き落とし | rear pull down | PULL |
| sokubiotoshi | 素首落とし | head chop down | PULL |
| ushiromotare | 後ろもたれ | backward lean out | OTHER |
| isamiashi | 勇み足 | forward step out (non-technique) | OTHER |
| koshikudake | 腰砕け | inadvertent collapse (non-technique) | OTHER |
| tsukite | つき手 | hand down (non-technique) | OTHER |
| tsukihiza | つきひざ | knee down (non-technique) | OTHER |
| fumidashi | 踏み出し | rear step out (non-technique) | OTHER |
| hansoku | 反則 | disqualification | OTHER |
| fusen | 不戦 | win/loss by default | excluded |

Notes:
- 82 techniques plus 5 non-techniques, `hansoku` and `fusen`. Phase 1 confirms the count and every spelling.
- Old-era data may contain techniques retired before 2001 (for example `abisetaoshi` variants or `kozumatori` spellings). Unknown keys keep their raw value, display as-is and count as OTHER.
- Japanese names from osada map through the Kanji column. Prefixes such as `決まり手：` and full-width spaces are stripped first.

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

## Appendix D: Snapshot file schema (draft, schema version 1)

`basho/YYYYMM.json.zst` (decompressed):

```json
{
  "schema": 1,
  "id": "202609",
  "start": "2026-09-13",
  "end": "2026-09-27",
  "location": null,
  "yusho": {"makuuchi": 8850, "juryo": 66},
  "sansho": [{"type": "kanto", "rikishi": 615}],
  "divisions": {
    "makuuchi": {
      "banzuke": [
        {"r": 8850, "rank": "Y1e", "rv": 101, "en": "Onosato", "jp": "大の里",
         "res": "WWWLWWWWWWWWLLW", "opp": [9, 0], "km": ["hatakikomi"]}
      ],
      "bouts": [
        [15, 1, 49, "M17e", 8865, "J2w", "w", "oshidashi", 0]
      ]
    },
    "juryo": {"banzuke": [], "bouts": []}
  },
  "measurements": {"8850": [190, 188]}
}
```

- `res`: one character per day: `W` win, `L` loss, `w` fusen win, `l` fusen loss, `A` absent, `D` draw, `-` no entry. `opp` and `km` have 15 entries each (0 / `""` when not applicable). The example is shortened.
- `bouts`: `[day, matchNo, eastId, eastRank, westId, westRank, winner ("e" | "w" | ""), kimarite, playoff (0/1)]`.
- `shikona` (`en`, `jp`) as of that tournament; `jp` already cleaned (§5.6).
- `rikishi.json.zst`: `[{"id","nsk","sdb","en","jp","heya","birth","from","debut","retired"}]`.
- `manifest.json`: `{"schema":1,"generator":"basho-data vX","generatedAt":"…","source":"sumo-api.com","basho":["196001",…],"sha256":{"basho/196001.json.zst":"…",…}}`.

---

## Appendix E: Live recorder for Kyushu 2026

`basho-data record [--out DIR] [--interval 10m] [--from 14:00] [--until 19:15] [--tournament YYYYMM] [--once]`

- **When it runs:** only on tournament days, between `--from` and `--until` (JST). On other days it logs one line and exits, so launchd can start it every day.
- **Which tournament:** the tournament is this month's, or the upcoming one in an even month. Its start date comes from sumo-api `/basho/{id}`, falling back to the second-Sunday estimate.
- **What each tick saves** into `DIR/<YYYYMMDD>/<HHMM>/`:
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

| Band | Y | O | S | K | M1–5 | M6–10 | M11–17 | J1–7 | J8–14 |
|---|---|---|---|---|---|---|---|---|---|
| Rating | 1750 | 1700 | 1650 | 1625 | 1575 | 1550 | 1525 | 1475 | 1450 |

Search ranges:

| Parameter | Values |
|---|---|
| `K` | 12, 16, 20, 24, 32 |
| `K_new` | 24, 32, 40, 48 |
| bouts counted as "new" | 15, 30, 45 |
| `r` (pull after a missed tournament) | 0, 0.1, 0.2, 0.3, 0.5 |
| warm-up table scale (spread around 1500) | 0.5, 1.0, 1.5 |

That is 900 combinations × about 210,000 bouts, a few seconds in Go. The best combination by log loss on the evaluation set wins; ties go to the smaller `K`. The report in `docs/rating.md` lists the top 5 combinations with all metrics and both baselines.
