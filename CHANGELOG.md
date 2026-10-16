# Changelog

All notable changes to basho. Versions follow [semantic
versioning](https://semver.org): a patch release ships new data only.

## Unreleased (v1.0.0)

The first release, private and installed with `make install`.

- Browse every Makuuchi and Juryo tournament since January 1960 from the
  data bundled in the binary (through Aki 2026), offline.
- Day view with the day's recap, the VS screen (tale of the tape, head-to-head,
  moveset, win chance, portraits), standings with the yusho race and the
  results grid, and a wrestler's career page.
- The current tournament from osada.us and sumo-api, with live mode during
  the broadcast and spoiler-free mode for watching later.
- Command line: `day`, `bout`, `standings`, `info`, `list`, `ratings`,
  `wrestler`, `watch`, `credits` and `config`, each with `--json`.
- Names in romaji, kanji or both (`ui.names`); `--ascii` and `--no-color`.
