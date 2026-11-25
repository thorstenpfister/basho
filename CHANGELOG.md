# Changelog

All notable changes to basho. Versions follow [semantic
versioning](https://semver.org): a patch release ships new data only.

## v1.2.0 (2026-11-25)

- Kyushu 2026's playoff is back (Onosato beat Wakamotoharu, watashikomi):
  it went missing when the tournament moved into the bundled data, whose
  source lists no playoffs. Playoffs now come from osada.us when a
  tournament is added.
- A playoff's VS heading reads "Bout 21 · KETTEI-SEN" instead of "Bout
  21/20".

## v1.1.0 (2026-11-25)

- Install with Homebrew: `brew tap thorstenpfister/basho
  https://github.com/thorstenpfister/basho`, then `brew install --cask
  thorstenpfister/basho/basho`. The cask brings the man page and the bash,
  zsh and fish completions.
- Releases are published on GitHub, with archives for macOS and Linux.
- MIT licence for the code.

## v1.0.0 (2026-11-25)

The first release, private and installed with `make install`.

- Browse every Makuuchi and Juryo tournament since January 1960 from the
  data bundled in the binary (through Kyushu 2026), offline.
- Day view with the day's recap, the VS screen (tale of the tape, head-to-head,
  moveset, win chance, portraits), standings with the yusho race and the
  results grid, and a wrestler's career page.
- The current tournament from osada.us and sumo-api, with live mode during
  the broadcast and spoiler-free mode for watching later. Live mode takes
  each result from whichever source has it first, and was checked against
  recordings of Kyushu 2026, playoff included.
- Command line: `day`, `bout`, `standings`, `info`, `list`, `ratings`,
  `wrestler`, `watch`, `credits` and `config`, each with `--json`.
- Names in romaji, kanji or both (`ui.names`); `--ascii` and `--no-color`.
