# Manual QA pass

The QA pass (SPEC §17, Phase 16), which is also Phase 12's acceptance check: the whole list in Ghostty and one other terminal (Terminal.app or iTerm2), each in a light and a dark theme. That makes four runs. Tick a box only when the item holds in every run, and note anything else under **Findings**.

| Run | Terminal | Theme | Size (`tput cols; tput lines`) | Date | Result |
|---|---|---|---|---|---|
| 1 | Ghostty | dark | | | |
| 2 | Ghostty | light | | | |
| 3 | | dark | | | |
| 4 | | light | | | |

Setup: `make build` (or `make install`), then for a clean first run:

```sh
export XDG_CONFIG_HOME=/tmp/basho-qa/config XDG_CACHE_HOME=/tmp/basho-qa/cache XDG_STATE_HOME=/tmp/basho-qa/state
rm -rf /tmp/basho-qa
```

Settings for single items go in `/tmp/basho-qa/config/basho/config.toml`; `basho config path` prints the path and `basho config show` what is in effect.

## Start and layout

- [ ] `basho` opens the current tournament's latest day within a blink, with the picker underneath (`esc`).
- [ ] The theme matches the terminal: readable text, visible rules and box borders, no washed-out dim text.
- [ ] Resizing through 60, 90 and 130 columns switches compact, normal and wide layouts; below 60×20, "Terminal too small".
- [ ] The status bar shows the source and age, and `? help`.

## Day view

- [ ] `←`/`→`, `1`–`0`, `!`–`%` and `g13` change the day; days without bouts can't be selected.
- [ ] `j`/`k` and the mouse wheel move the cursor (▶).
- [ ] `d` switches Makuuchi and Juryo; `p` toggles the win-chance column.
- [ ] Wide layout: the yusho race panel beside the table.
- [ ] `r` opens the recap over the table; `←`/`→` move the day with it; `r` or `esc` close it.
- [ ] `R` refreshes once, then says to wait (30 s).

## VS screen

- [ ] `enter` slides it in (animation); `ui.animations = false` and the compact layout show it without.
- [ ] Tale of the tape: height and weight as plain text, STYLE bars (textures █ ▓ ▒ ░ ▚ in colour), SIGNATURE, RATING with ▲/▼, WIN CHANCE.
- [ ] Portraits: 20 px above the names, East's right-aligned and West's left-aligned with their columns, "photo 2024" under each. A silhouette for a wrestler without a photo, nothing when neither has one.
- [ ] Portraits need about 40 rows: shrink the terminal below that and they go, grow it and they come back. Never in the compact layout, with `--ascii`, without colours, with `ui.portraits = false` or with `--no-portraits` (the wrestler screen too); the help's Data page says how.
- [ ] A portrait never appears and then vanishes while the bout loads.
- [ ] `[`/`]` step through bouts and across days; the first and last bout say so.
- [ ] `m` moveset panel, `w` cycles its window; `v` head-to-head history.
- [ ] `1`/`2` open the East and West wrestlers' careers.
- [ ] Result panel: winner, technique in romaji, kanji and English; KINBOSHI and FUSEN stamps where they apply.

## Wrestler screen

- [ ] From a VS screen, `1` and `2` open the careers; the status bar reads "… · career through Aki 2026".
- [ ] Header: the 32 px portrait (when there is a photo), the name, then one labelled row each: RANK, HEYA, FROM, AGE, HEIGHT, WEIGHT, YUSHO, PRIZES, KINBOSHI, RATING with its peak, STYLE, SIGNATURE.
- [ ] RANK and RATING sparklines; a long career (Hakuho) says "last N of M".
- [ ] The table: newest first, records coloured, YUSHO and prizes marked, "as Kisenosato"-style notes for earlier names; `j`/`k`, the wheel, `g`/`G` move; it scrolls and the hints stay on screen.
- [ ] `enter` opens that tournament's day view in the right division, on the wrestler's last bout day; `esc` comes back.
- [ ] Opened from an older tournament (a bout of 2024), the career ends there.
- [ ] Spoiler-free: the tournament seen from stops at the horizon (record with "…", no YUSHO), with the "as of … (spoiler-free)" row.
- [ ] Standings grid: `i` opens the selected wrestler's career.

## Standings and picker

- [ ] `s` opens the standings after the shown day; `tab` switches race and grid; `←`/`→` move the day.
- [ ] Grid: `j`/`k` and the wheel move the cursor; `enter` opens a wrestler's results; KK/MK and 休 shown.
- [ ] Race: the tail below −2 is one line until `enter` expands it.
- [ ] Picker: `/` filters by year, name and month; `enter` opens; cancelled tournaments marked; yusho winners shown.

## Names (`ui.names`)

- [ ] `both` (default): romaji first, kanji alongside.
- [ ] `en`: no kanji or kana anywhere, 結び and 休 become "musubi" and "kyujo"; layouts unchanged.
- [ ] `jp`: kanji first everywhere (day view, standings, VS screen, career), romaji beside on the VS screen; columns stay aligned.

## Spoiler-free

- [ ] `S` hides every result past the horizon as `?`, everywhere: day view, VS screen, standings, recap, picker yusho, ratings, careers.
- [ ] VS screen `space` reveals the bout, and its result flashes in; `ctrl+r` in the day view asks before revealing up to the cursor.
- [ ] The horizon survives quitting and starting again (`basho --no-spoilers`).

## Help

- [ ] `?` opens the overlay; `←`/`→` or `tab` switch the five pages; the current one is in brackets.
- [ ] At 80×24 the long pages scroll with `j`/`k` and the wheel, and the footer says where you are.
- [ ] The Rating page shows the calibration numbers; the Data page credits both sources and counts the portraits.
- [ ] The Keys page lists `1`/`2` on the VS screen and `i` in the standings.

## Plain modes

- [ ] `basho --ascii`: no symbol outside ASCII on any screen (kanji names may stay), layouts unchanged; sparklines as `_ . - = #`.
- [ ] `NO_COLOR=yes basho` and `basho --no-color`: no colour; the STYLE bar shows letters; bold and dim still show; no portraits.
- [ ] `ui.mouse = false`: the wheel does nothing, and text can be selected with the mouse.

## Live mode (during a tournament, 14:00–18:45 JST)

- [ ] The status bar shows ● LIVE with the next poll; new results arrive with NEW badges.
- [ ] `L` turns it off and on; `R` polls now.
- [ ] Outside the window: ◌ and when it opens.
- [ ] `basho watch` prints each result as it comes in; `--json` one event per line.

## CLI

- [ ] `basho day`, `bout 13 19 -t 202609`, `standings --grid`, `info`, `list --from 2025`, `ratings`, `credits`: readable tables in the terminal, plain when piped (`| cat`).
- [ ] `basho wrestler Onosato`, `basho wrestler 大の里` and `basho wrestler 8850` show the same career; `basho wrestler Takanohana` lists two wrestlers and exits 2; `basho wrestler Nobody` exits 3.
- [ ] `basho credits --portraits` lists every photo with author and licence.
- [ ] `--json` on each prints the envelope.
- [ ] `basho config path` prints the path; `basho config show` marks changed settings with `# default:`.
- [ ] A broken `config.toml` names the file; `basho config path` still works.
- [ ] `basho info next --offline` explains itself and exits 4; error paths are printed as they are (no capitalised path).
- [ ] `basho version` shows the version and the build date.

## Data tools (once, not per run)

- [ ] `make portraits-preview` rebuilds only the overrides in seconds; with a work cache less than a week old it makes no requests.
- [ ] `make portraits` shows its plan, asks, then a progress line per phase; `bin/basho` afterwards has the new set.
- [ ] `make vhs` records `docs/demo.gif` (needs VHS).

## Findings

<!-- Run, screen, what happened, what was expected. -->
