# Releasing

A release is a tag, published with `make release`: GoReleaser builds the
archives, creates the GitHub release with the tag's CHANGELOG entry as its
notes, and commits the Homebrew cask (`Casks/basho.rb`) to `main` (SPEC
§14.3). It needs `goreleaser` and `gh` (logged in, `gh auth status`).

Versions: a **patch** (`v1.0.1`) ships new data only, a **minor** (`v1.1.0`)
new features or fixes. The CHANGELOG has an entry for each.

## Data release after a tournament (about 15 minutes)

A tournament counts as finished two days after its last day, once it has a
yusho winner (§7.2). Before that, `make snapshot` skips it.

1. **Update the snapshot.** It shows the plan, asks, then fetches about 34
   requests per new tournament at 1 per second, and verifies:

   ```sh
   make snapshot
   ```

   `verify` must end without problems. A new oddity in the source data goes
   in `data/snapshot/known-issues.txt` with a `#` reason (§4.3).

   Playoff bouts come from osada.us, which sumo-api lacks; the log says
   "202701: 1 playoff bout from osada" when there was one. If it says "no
   playoff bouts from osada: …" for a tournament that had a playoff (osada
   was down, say), add them while osada still serves that tournament (until
   the next banzuke, about two weeks):

   ```sh
   bin/basho-data snapshot playoffs 202701
   ```

2. **Portraits** (optional, for wrestlers new to the top divisions). Keep
   `.cache/basho-data` between runs, so only new photos are fetched:

   ```sh
   make portraits CONTACT=you@example.org
   ```

   Look over `.cache/basho-data/portraits/sheet.png` before committing.

3. **Check and commit:**

   ```sh
   make check
   git add data
   git commit -m "data: snapshot through 202701"
   ```

   A test that fails only with the new data has taken some tournament as
   "after the snapshot"; take the snapshot's newest + 1 from
   `service.Bundled()` instead of naming one.

   The rating needs no new calibration: it is evaluated up to the
   tournament named in `data/rating/params.json`, so new data leaves its
   test alone. Recalibrate (`make calibrate`) only to change the model.

4. **CHANGELOG:** add the version with one line, "Data through Hatsu 2027",
   and commit it.

5. **Tag, push, publish, install:**

   ```sh
   git tag -a v1.0.1 -m "v1.0.1: data through Hatsu 2027"
   git push origin main v1.0.1
   make release                 # asks first; YES=1 skips the question
   brew upgrade --cask basho    # or: make install
   basho version
   ```

   `basho version` must show the tag (not a commit ID) and the new data
   line, e.g. `data 2027-01-27 (196001–202701)`. A commit ID means the tag
   is not on the commit you built. `make release` refuses an untagged HEAD,
   a dirty tree, a tag not yet pushed, or a tag without a CHANGELOG entry
   `## v1.0.1 (…)`; afterwards it pulls the cask commit GoReleaser made.

## Code release

1. `make check`, then the parts of `docs/qa.md` the change touches.
2. CHANGELOG entry; commit.
3. `make release-snapshot` as a dry run: archives for the four targets,
   completions and man page, checksums, all in `dist/`. Two runs on the
   same commit give the same checksums.
4. Tag, push, publish and install as in step 5 above.

## Completions and man page

`make install` installs only the binary. Once per machine, after an install
that changed commands or flags:

```sh
# zsh, with oh-my-zsh; otherwise any directory on $fpath
basho completion zsh > ~/.oh-my-zsh/custom/completions/_basho
# then open a new shell; if completion doesn't appear: rm ~/.zcompdump* && exec zsh
```

The man page: `basho man | mandoc -a` shows it without installing. The release archives carry both, made by
`scripts/completions.sh`.
