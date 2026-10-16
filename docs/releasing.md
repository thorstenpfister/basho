# Releasing

basho v1 is installed from source with `make install`; the repository is
private and nothing is published (SPEC §14.2). A release is a tag, so that
`basho version` names it. Publishing to GitHub and the Homebrew cask come
with Phase 18 (§14.3).

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
   git commit -m "data: snapshot through 202611"
   ```

   The rating needs no new calibration: it is evaluated up to the
   tournament named in `data/rating/params.json`, so new data leaves its
   test alone. Recalibrate (`make calibrate`) only to change the model.

4. **CHANGELOG:** add the version with one line, "Data through Kyushu 2026",
   and commit it.

5. **Tag, push, install:**

   ```sh
   git tag -a v1.0.1 -m "v1.0.1: data through Kyushu 2026"
   git push origin main v1.0.1
   make install
   basho version
   ```

   `basho version` must show the tag (not a commit ID) and the new data
   line, e.g. `data 2026-11-26 (196001–202611)`. A commit ID means the tag
   is not on the commit you built.

## Code release

1. `make check`, then the parts of `docs/qa.md` the change touches.
2. CHANGELOG entry; commit.
3. `make release-snapshot` as a dry run: archives for the four targets,
   completions and man page, checksums, all in `dist/`. Two runs on the
   same commit give the same checksums.
4. Tag, push and install as in step 5 above.

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
