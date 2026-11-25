# basho

A terminal app for browsing grand sumo tournaments: day by day, bout by bout,
with a fighting-game-style preview of every bout, standings after any day and
a live mode during tournaments.

![basho: the day view, a bout's tale of the tape and a wrestler's career](docs/demo.gif)

**Status:** in development. [`SPEC.md`](SPEC.md) describes what basho does,
how it is built, and the implementation plan.

## Install

With [Homebrew](https://brew.sh), on macOS or Linux:

```sh
brew tap thorstenpfister/basho https://github.com/thorstenpfister/basho
brew install --cask thorstenpfister/basho/basho
basho                        # the current tournament
```

The cask installs the man page and the bash, zsh and fish completions too.
Update with `brew upgrade --cask basho`. The tap needs its URL, because this
repository isn't named `homebrew-…`. basho isn't signed; the cask removes
macOS's quarantine flag so it runs without a Gatekeeper prompt.

With Go (the version in `go.mod`):

```sh
go install github.com/thorstenpfister/basho/cmd/basho@latest
```

The [releases](https://github.com/thorstenpfister/basho/releases) have
archives for macOS and Linux (arm64 and amd64), with completions and the man
page. From a clone, `make install`.

[`CHANGELOG.md`](CHANGELOG.md) says what changed; the data bundled in basho
is updated after each tournament.

## Development

Requirements: the Go version in `go.mod` and
[golangci-lint](https://golangci-lint.run) v2. `gofumpt` and `govulncheck` run
via `go tool`.

```sh
make help      # all targets
make check     # run before committing
make install   # basho with version info
```

[`docs/charm-v2-notes.md`](docs/charm-v2-notes.md) lists the Charm v2 APIs
basho relies on; v2 differs a lot from v1.

## Data sources

- [sumo-api.com](https://www.sumo-api.com): tournaments, rankings, bouts and
  wrestler data; free and run by a volunteer.
- [osada.us](https://osada.us) ([o-sumo](https://github.com/dai/o-sumo)): live
  results of the current tournament.
- Both derive from the official results of the
  [Japan Sumo Association](https://www.sumo.or.jp).
- Portraits: photos from [Wikimedia Commons](https://commons.wikimedia.org),
  cropped, downscaled and colour-reduced, each under its own licence
  (mostly CC BY-SA). [`data/portraits/CREDITS.md`](data/portraits/CREDITS.md)
  and `basho credits --portraits` credit every photo.

Usage policies and how basho stays polite: SPEC.md §4 and §7.

## Licence

The code is under the [MIT licence](LICENSE). It does not cover:

- the portraits in `data/portraits/`: each keeps its photo's licence,
  listed in [`CREDITS.md`](data/portraits/CREDITS.md);
- the tournament data in `data/snapshot/`, built from sumo-api.com;
- the recorded responses in `testdata/`, which belong to sumo-api.com and
  osada.us ([`testdata/README.md`](testdata/README.md)).
