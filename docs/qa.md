# Manual QA pass

The QA pass (SPEC §17, Phase 16), which is also Phase 12's acceptance check: the whole list in Ghostty and one other terminal (Terminal.app or iTerm2), each in a light and a dark theme. That makes four runs. Tick a box only when the item holds in every run, and note anything else under **Findings**.

| Run | Terminal | Theme | Size (`tput cols; tput lines`) | Date | Result |
|---|---|---|---|---|---|
| 1 | Ghostty | dark | 183×52 | 2026-10-16 | pass after three findings, all fixed |
| 2 | Ghostty | light | | | not run |
| 3 | Terminal.app | dark | | | not run |
| 4 | Terminal.app | light | | 2026-10-16 | pass |

The 2026 pass stopped after runs 1 and 4, one of each terminal and theme, by the owner's call. Live mode waits for Kyushu 2026, and the data tools were not run in it.

Setup: `make build` (or `make install`), then for a clean first run:

```sh
export XDG_CONFIG_HOME=/tmp/basho-qa/config XDG_CACHE_HOME=/tmp/basho-qa/cache XDG_STATE_HOME=/tmp/basho-qa/state
rm -rf /tmp/basho-qa
```

Settings for single items go in `/tmp/basho-qa/config/basho/config.toml`; `basho config path` prints the path and `basho config show` what is in effect.

## Start and layout

- [x] `basho` opens the current tournament's latest day within a blink, with the picker underneath (`esc`).
- [x] The theme matches the terminal: readable text, visible rules and box borders, no washed-out dim text.
- [x] Resizing through 60, 90 and 130 columns switches compact, normal and wide layouts; below 60×20, "Terminal too small".
- [x] The status bar shows the source and age, and `? help`.

## Day view

- [x] `←`/`→`, `1`–`0`, `!`–`%` and `g13` change the day; days without bouts can't be selected.
- [x] `j`/`k` and the mouse wheel move the cursor (▶).
- [x] `d` switches Makuuchi and Juryo; `p` toggles the win-chance column.
- [x] Wide layout: the yusho race panel beside the table.
- [x] `r` opens the recap over the table; `←`/`→` move the day with it; `r` or `esc` close it.
- [x] `R` refreshes once, then says to wait (30 s).

## VS screen

- [x] `enter` slides it in (animation); `ui.animations = false` and the compact layout show it without.
- [x] Tale of the tape: height and weight as plain text, STYLE bars (textures █ ▓ ▒ ░ ▚ in colour), SIGNATURE, RATING with ▲/▼, WIN CHANCE.
- [x] Portraits: 20 px above the names, East's right-aligned and West's left-aligned with their columns, "photo 2024" under each. A silhouette for a wrestler without a photo, nothing when neither has one.
- [x] Portraits need about 40 rows: shrink the terminal below that and they go, grow it and they come back. Never in the compact layout, with `--ascii`, without colours, with `ui.portraits = false` or with `--no-portraits` (the wrestler screen too); the help's Data page says how.
- [x] A portrait never appears and then vanishes while the bout loads.
- [x] `[`/`]` step through bouts and across days; the first and last bout say so.
- [x] `m` moveset panel, `w` cycles its window; `v` head-to-head history.
- [x] `1`/`2` open the East and West wrestlers' careers.
- [x] Result panel: winner, technique in romaji, kanji and English; KINBOSHI and FUSEN stamps where they apply.

## Wrestler screen

- [x] From a VS screen, `1` and `2` open the careers; the status bar reads "… · career through Aki 2026".
- [x] Header: the 32 px portrait (when there is a photo), the name, then one labelled row each: RANK, HEYA, FROM, AGE, HEIGHT, WEIGHT, YUSHO, PRIZES, KINBOSHI, RATING with its peak, STYLE, SIGNATURE.
- [x] RANK and RATING sparklines; a long career (Hakuho) says "last N of M".
- [x] The table: newest first, records coloured, YUSHO and prizes marked, "as Kisenosato"-style notes for earlier names; `j`/`k`, the wheel, `g`/`G` move; it scrolls and the hints stay on screen.
- [x] `enter` opens that tournament's day view in the right division, on the wrestler's last bout day; `esc` comes back.
- [x] Opened from an older tournament (a bout of 2024), the career ends there.
- [x] Spoiler-free: the tournament seen from stops at the horizon (record with "…", no YUSHO), with the "as of … (spoiler-free)" row.
- [x] Standings grid: `i` opens the selected wrestler's career.

## Standings and picker

- [x] `s` opens the standings after the shown day; `tab` switches race and grid; `←`/`→` move the day.
- [x] Grid: `j`/`k` and the wheel move the cursor; `enter` opens a wrestler's results; KK/MK and 休 shown.
- [x] Race: the tail below −2 is one line until `enter` expands it.
- [x] Picker: `/` filters by year, name and month; `enter` opens; cancelled tournaments marked; yusho winners shown.

## Names (`ui.names`)

- [x] `both` (default): romaji first, kanji alongside.
- [x] `en`: no kanji or kana anywhere, 結び and 休 become "musubi" and "kyujo"; layouts unchanged.
- [x] `jp`: kanji first everywhere (day view, standings, VS screen, career), romaji beside on the VS screen; columns stay aligned.

## Spoiler-free

- [x] `S` hides every result past the horizon as `?`, everywhere: day view, VS screen, standings, recap, picker yusho, ratings, careers.
- [ ] `S` names how far the tournament on screen is seen ("Spoiler-free: Aki 2026 seen through …" or "nothing seen yet"). New after run 1.
- [x] VS screen `space` reveals the bout, and its result flashes in; `ctrl+r` in the day view asks before revealing up to the cursor.
- [x] The horizon survives quitting and starting again (`basho --no-spoilers`).

## Help

- [x] `?` opens the overlay; `←`/`→` or `tab` switch the five pages; the current one is in brackets.
- [x] At 80×24 the long pages scroll with `j`/`k` and the wheel, and the footer says where you are.
- [x] The Rating page shows the calibration numbers; the Data page credits both sources and counts the portraits.
- [x] The Keys page lists `1`/`2` on the VS screen and `i` in the standings.

## Plain modes

- [x] `basho --ascii`: no symbol outside ASCII on any screen (kanji names may stay), layouts unchanged; sparklines as `_ . - = #`.
- [x] `NO_COLOR=yes basho` and `basho --no-color`: no colour; the STYLE bar shows letters; bold and dim still show; no portraits.
- [x] `ui.mouse = false`: the wheel does nothing, and text can be selected with the mouse.

## Live mode (during a tournament, 14:00–18:45 JST)

- [ ] The status bar shows ● LIVE with the next poll; new results arrive with NEW badges.
- [ ] `L` turns it off and on; `R` polls now.
- [ ] Outside the window: ◌ and when it opens.
- [ ] `basho watch` prints each result as it comes in; `--json` one event per line.

## CLI

- [x] `basho day`, `bout 13 19 -t 202609`, `standings --grid`, `info`, `list --from 2025`, `ratings`, `credits`: readable tables in the terminal, plain when piped (`| cat`).
- [x] `basho wrestler Onosato`, `basho wrestler 大の里` and `basho wrestler 8850` show the same career; `basho wrestler Takanohana` lists two wrestlers and exits 2; `basho wrestler Nobody` exits 3.
- [x] `basho credits --portraits` lists every photo with author and licence.
- [x] `--json` on each prints the envelope.
- [x] `basho config path` prints the path; `basho config show` marks changed settings with `# default:`.
- [x] A broken `config.toml` names the file; `basho config path` still works.
- [x] `basho info next --offline` explains itself and exits 4; error paths are printed as they are (no capitalised path).
- [x] `basho version` shows the version and the build date.

## Data tools (once, not per run)

- [ ] `make portraits-preview` rebuilds only the overrides in seconds; with a work cache less than a week old it makes no requests.
- [ ] `make portraits` shows its plan, asks, then a progress line per phase; `bin/basho` afterwards has the new set.
- [x] `make vhs` records `docs/demo.gif` (needs VHS; without Chrome installed, VHS downloads a headless Chromium of about 285 MB to `~/.cache/rod`).

## Findings

<!-- Run, screen, what happened, what was expected. -->

- **Run 1, picker:** in normal mode the picker showed no yusho until `S` was turned on and off again. The picker under the day view was never started. Fixed in 63e2fd1.
- **Run 1, day view:** `S` on day 4 of Aki 2026 hid nothing. That tournament was revealed to the end in the state file (the run used the normal state, not `/tmp/basho-qa`), so not a bug; but nothing said so. `S` now names the horizon (a2d120f).
- **Run 1, VS screen:** at 183 columns the head-to-head column cut techniques ("kotena…") with about 30 columns unused. It now takes the width the tape leaves (661edf8).
