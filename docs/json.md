# basho JSON output

Every data command takes `--json` and prints one JSON document to stdout (SPEC §11). The schema is versioned; golden files in `internal/cli/testdata/` lock it down, so a change to this page goes with a change to them.

## Envelope

```json
{
  "schemaVersion": 1,
  "kind": "day",
  "provenance": {
    "source": "snapshot",
    "fetchedAt": "2026-10-14T06:36:34Z",
    "sourceUpdatedAt": null,
    "stale": false
  },
  "data": { … }
}
```

| Field | Meaning |
|---|---|
| `schemaVersion` | `1`. Raised on any incompatible change. |
| `kind` | `list`, `info`, `day`, `bout`, `standings`, `ratings`, `credits`, `config` or `config_path`: says what `data` is. |
| `provenance.source` | `snapshot` (embedded data), `sumo-api` (any part came from the network), `cache:sumo-api` (all from the disk cache), `calendar` (no data needed, e.g. a cancelled tournament), `basho` (static text). |
| `provenance.fetchedAt` | UTC. The snapshot's build time, or the oldest response used. |
| `provenance.sourceUpdatedAt` | The source's own update time when it gives one (osada.us, Phase 9); otherwise `null`. |
| `provenance.stale` | `true` when an expired cache entry stood in for a source that failed. |

## Conventions

- Field names are camelCase. Enums are stable lowercase strings; multi-word values use `_` (`fusen_loss`, `in_progress`).
- Tournaments are `"YYYYMM"` strings, dates `"YYYY-MM-DD"` (the day in Japan).
- Probabilities and shares are rounded to 4 decimals, ratings to 2.
- A missing value is `null` (objects) or an empty array, never an absent field, except where marked optional.

### Shared types

**Rank:** `{"title": "maegashira", "number": 17, "side": "east", "short": "M17e"}`. `title` is `yokozuna`, `ozeki`, `sekiwake`, `komusubi`, `maegashira`, `juryo` or `makushita` (a visitor in a Juryo bout); `side` is `east` or `west`.

**Wrestler:** `{"id": 8850, "shikona": {"en": "Onosato", "jp": "大の里"}, "rank": Rank}`. `id` is sumo-api's. `shikona` is the name as of that tournament; `jp` is omitted when unknown (older records), `rank` when there is none.

**Record:** `{"wins": 11, "losses": 1, "absences": 0, "draws": 0, "text": "11-1"}`. Fusen wins and losses count as wins and losses; playoffs never count.

**Technique:** `{"kimarite": "shitatenage", "kanji": "下手投げ", "english": "underarm throw"}`. `kimarite` is the canonical romaji key; `kanji` and `english` are omitted for values not in basho's table.

**Outcome** (in standings results): `pending`, `win`, `loss`, `fusen_win`, `fusen_loss`, `absent`, `draw`, `unknown`.

## Kinds

### `list`

An array, newest first: `{"basho", "name", "venue", "status", "yusho": Winner | null, "inSnapshot"}`. `status` is `upcoming`, `in_progress`, `finished` or `cancelled`; `yusho` is the Makuuchi winner when known.

### `info`

`{"basho", "name", "venue", "status", "start", "end", "yusho": [Winner], "prizes": [Winner]}`. `start` and `end` are `null` for a cancelled tournament. A Winner is a Wrestler with `division` (yusho) or `prize` (`Kanto-sho`, `Shukun-sho`, `Gino-sho`) and the final `record` (`null` for a lower division's winner). Lower divisions' winners appear only when the wrestler index names them.

### `day`

`{"basho", "name", "venue", "division", "day", "date", "status", "bouts": [Bout], "playoffs": [Bout], "kyujo": [Wrestler]}`. `division` is `makuuchi` or `juryo`; `status` is `not_published`, `scheduled`, `in_progress` or `complete`.

A Bout: `{"matchNo", "east": Side, "west": Side, "winner": "east" | "west" | null, "technique": Technique | null, "fusen", "kinboshi", "musubi", "winChance"}`. A Side is a Wrestler with `before` and `after` Records, both `null` for a visitor from a lower division, who has no record in this one. `winChance` is the east side's chance before the bout, `null` when the rating is hidden (V9).

### `bout`

The tale of the tape, as of just before the bout: nothing in it counts the bout itself except `result`.

`{"basho", "name", "division", "day", "matchNo", "of", "musubi", "playoff", "east": TaleSide, "west": TaleSide, "headToHead", "matchup", "winChance", "result"}`

A TaleSide is a Wrestler with:

| Field | Meaning |
|---|---|
| `heya`, `shusshin` | Stable and hometown, current values (sumo-api has no history of them) |
| `profile` | `false` when the wrestler has no profile; `heya`, `shusshin`, `age`, `height` and `weight` are then empty |
| `age` | At the tournament's start; `0` when unknown |
| `height`, `weight` | cm and kg, the latest known; `measurementsLatest` is `true` outside the current tournament |
| `now` | Optional: today's shikona when it differs |
| `yusho` | Division name → count, from tournaments over before the cutoff |
| `sansho` | Prize name → count |
| `record` | Record going into the bout; `null` for a visitor from a lower division |
| `last5` | `{"results": [{"basho", "day", "opponent", "won", "fusen", "kimarite"}], "new"}`, oldest first, across tournaments; `new` when the career is shorter |
| `kinboshi` | `{"count", "thisBasho"}` |
| `moveset` | `{"window", "tournaments", "wins", "losses", "winShare", "lossShare", "topWins", "topLosses", "signature", "signatureShare", "fighterType", "enough"}`. Shares are keyed `push`, `belt`, `throw`, `pull`, `other`; counts are `{"kimarite", "count"}`; `fighterType` is empty and `enough` false below 10 wins |
| `rating` | `{"value", "change", "provisional"}`, `null` when hidden; `change` is since the tournament's start |

`headToHead` is `{"eastWins", "westWins", "eastTop", "westTop", "meetings"}` with the last 3 meetings newest first (`{"basho", "day", "winner", "kimarite", "eastRank", "westRank", "playoff"}`). `matchup` is `{"attacker": "east" | "west", "category", "winShare", "lossShare"}` or `null`. `result` is `null` while undecided, otherwise `{"winner", "technique", "kinboshi", "fusen", "upset", "east", "west"}` with the Records after the bout.

### `standings`

`{"basho", "name", "division", "day", "rows", "yushoDecided", "playoff", "yusho", "kachiKoshi", "makeKoshi", "kyujo"}`. `rows` are in standings order (wins, then losses and absences, then rank); each is a Wrestler with `record`, `remaining`, `band` (wins behind the leader, `3` for the rest), `inContention`, `kachiKoshi`, `makeKoshi`, `newToday`, `kyujo` and `results` (15 Outcomes, `pending` after the day). `yusho` is the winner once the tournament is final, also after a playoff.

### `ratings`

`{"basho", "shown", "rows"}`: each row is a Wrestler with `value`, `change` and `provisional`, highest first. `shown` is `false` when the rating did not pass calibration.

### `credits`

`{"text", "portraits"}`: the credits as text, and every portrait's photo as `{"id", "name", "file", "author", "licence", "licenceUrl", "sourceUrl", "year"}` (`[]` without portraits). `licenceUrl` and `year` are left out when unknown.

### `config`

`basho config show --json`: the configuration in effect, one object per section with the file's keys (`{"ui": {"theme": "auto", …}, "live": {"interval": "3m", …}, …}`). Durations and times of day are strings as the file writes them. The session switches (`source`, `offline`, `debug`) are not part of it.

### `config_path`

`basho config path --json`: `{"path", "exists"}`, the config file in use and whether it is there.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | Internal error |
| 2 | Usage error (bad flag, argument, tournament or config) |
| 3 | Not found, or not available yet (a future tournament, an unpublished day, a bout number out of range) |
| 4 | Every source failed and nothing is cached (also `--offline` without a cached copy) |

Errors go to stderr as text, also with `--json`.
