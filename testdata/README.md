# Test data

Recorded responses that the tests replay; nothing here is fetched at test
time. The data belongs to its sources and is not covered by basho's MIT
licence.

- `sumoapi/`: responses from [sumo-api.com](https://www.sumo-api.com),
  recorded 2026-10-13 with `make fixtures` (SPEC §13).
- `osada/`: responses from [osada.us](https://osada.us)
  ([o-sumo](https://github.com/dai/o-sumo)), recorded 2026-10-13. osada.us
  scrapes the Japan Sumo Association's official pages; its usage policy
  allows search and AI input, **not AI training**, and asks that the source
  and time be cited. Each file's `updatedAt` gives its time.
- `live/`: Kyushu 2026 as `basho-data record` saw it, from both sources:
  six ticks each on days 1, 5 and 15 (2026-11-08, 11-12 and 11-22), trimmed
  from the full recording (SPEC Appendix E). `meta.json` in each tick has
  the request times. `live/sumoapi/` holds Kyushu's banzuke, which the
  recorder does not fetch, fetched once on 2026-11-22.
- `snapshot/`: a three-tournament snapshot built from the sumo-api
  fixtures (196001, 202607, 202609).
