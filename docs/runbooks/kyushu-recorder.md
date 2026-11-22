# Runbook: live recorder on an always-on Mac (Kyushu 2026)

Records the raw osada.us and sumo-api.com responses during every tournament day
of Kyushu 2026 (8–22 November), as test fixtures for live mode and to measure
how quickly sumo-api.com publishes results (SPEC Appendix E, V1).

It runs unattended on an always-on Mac (called `<host>` below) as a
LaunchDaemon. **Ready by 7 November.**

## How it works

The recorder's behaviour is specified in SPEC Appendix E; the defaults are in
`basho-data record --help`. In short:

- launchd starts `basho-data record` every day at 05:30 local time, after every
  reboot, and again a minute after a run fails.
- The recorder looks up the tournament dates on sumo-api.com, falling back to
  the second-Sunday estimate if that fails.
  - **Not a tournament day:** it logs one line and exits.
  - **Tournament day:** it waits for the window (14:00–19:15 JST by default),
    then every 10 minutes saves five responses to
    `~/basho-live/YYYYMMDD/HHMM/` with a `meta.json`. That is 32 ticks a day
    with the defaults.
- Started late (for example after a reboot), it skips the past ticks and
  continues.

**Time zones:** the window is computed in JST, independent of the Mac's time
zone. On CET, as in November in central Europe, 14:00 JST is 06:00 local and
the 05:30 start is 13:30 JST on the same Japanese day. In another time zone,
any start time between 00:00 and 13:59 JST works; adjust `Hour` in the plist.

**Disk:** about 25 MB per day, about 380 MB for the tournament.

## Account

Install it under your own admin account on the Mac (called `<you>` below), not
under a shared or service account that other people or programs control:
they could change the binary and the recordings. The LaunchDaemon runs as that
user without anyone being logged in.

## 1. Checks on the Mac (once)

```sh
ssh <you>@<host>
uname -m                            # arm64 or x86_64, needed for step 2
pmset -g | grep -E ' sleep|autorestart'   # expect: sleep 0, autorestart 1
sudo systemsetup -gettimezone       # see "Time zones"
df -h ~                             # a few hundred MB free
```

If `sleep` is not 0, the Mac may sleep through the window. In that case run
`sudo pmset repeat wakeorpoweron MTWRFSU 05:20:00`, but check
`pmset -g sched` first, because `repeat` replaces any existing schedule.

## 2. Build and copy the binary (laptop)

```sh
cd ~/github/basho
git pull                       # the version you want to run
make build                     # bin/basho-data for this Mac's architecture
bin/basho-data --version
```

If the Mac reported `x86_64` in step 1, build for it instead:

```sh
GOARCH=amd64 make build        # same path, bin/basho-data
```

Copy it:

```sh
ssh <you>@<host> 'mkdir -p ~/basho/bin ~/basho-live ~/Library/Logs'
scp bin/basho-data <you>@<host>:basho/bin/basho-data
scp deploy/launchd/io.github.thorstenpfister.basho.record.plist <you>@<host>:basho/
```

`scp` does not set the quarantine flag, so Gatekeeper is not involved. If
macOS refuses to run the binary anyway, use
`xattr -d com.apple.quarantine ~/basho/bin/basho-data`.

## 3. Outgoing firewall (Mac, if it has one)

If the Mac runs an outgoing firewall that asks per connection, an unanswered
alert blocks the recorder:

- Add two explicit **allow** rules for `/Users/<you>/basho/bin/basho-data`:
  - `www.sumo-api.com`, TCP 443
  - `osada.us`, TCP 443
- DNS goes through the system resolver and needs no rule.

Every new copy of the binary has a new ad-hoc code signature. If the firewall
binds rules to the signature, re-confirm them after each copy in step 2.
**Don't copy a new binary during the tournament.** If you must, re-confirm the
rules before the next tick.

## 4. Smoke test (Mac)

Run one tick now, outside any window:

```sh
~/basho/bin/basho-data record --once --out /tmp/basho-smoke
cat /tmp/basho-smoke/*/*/meta.json
rm -rf /tmp/basho-smoke
```

Expect:

- five entries with `"status": 200` and no `"error"`
- `meta.json`, plus two files in `osada/` and three in `sumoapi/`
- a single line `… tick /tmp/basho-smoke/…: 5 requests, 0 failed`. The Kyushu
  banzuke is out (checked 26 October), so sumo-api has the real start date and
  there should be no `no start date from sumo-api` line. If there is one, run
  `curl -s https://www.sumo-api.com/api/basho/202611` and look for
  `"startDate":"2026-11-08…"`.
- `osada/banzuke.json` may still show the September banzuke (`九月場所`). osada
  switches around the start of the tournament, and the recorder saves whatever
  it serves.

An `error` mentioning a connection points to the firewall (step 3) or DNS. A
single `no such host` is usually a resolver hiccup; run the test again and
check that all five requests succeed.

## 5. Install the LaunchDaemon (Mac)

```sh
cd ~/basho
sed "s/__USER__/$USER/g" io.github.thorstenpfister.basho.record.plist > record.plist
plutil -lint record.plist
sudo install -m 644 -o root -g wheel record.plist /Library/LaunchDaemons/io.github.thorstenpfister.basho.record.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/io.github.thorstenpfister.basho.record.plist
```

`RunAtLoad` starts it immediately. On a non-tournament day it exits at once:

```sh
tail -3 ~/Library/Logs/basho-record.log
# … 202611: 2026-10-20 is not a tournament day
sudo launchctl print system/io.github.thorstenpfister.basho.record | grep -E 'state|last exit'
# state = not running, last exit code = 0
```

## 6. During the tournament (8–22 November)

Each morning the log shows the plan, then one line per tick:

```sh
ssh <you>@<host>
tail -f ~/Library/Logs/basho-record.log
# … 202611 day 1: recording 14:00–19:15 JST every 10m0s
# … tick /Users/<you>/basho-live/20261108/1400: 5 requests, 0 failed
```

Checks during or after a day:

```sh
D=$(TZ=Asia/Tokyo date +%Y%m%d)
ls ~/basho-live/$D | wc -l                              # 32 after 19:15 JST (defaults)
grep -l '"error"' ~/basho-live/$D/*/meta.json           # ticks with failed requests
grep -h '"status"' ~/basho-live/$D/*/meta.json | sort | uniq -c
```

A few failed requests are fine; the next tick tries again. A day with fewer
ticks means the run started late or stopped; launchd restarts a failed run
within a minute.

| Symptom | Likely cause | Action |
|---|---|---|
| No log line on a tournament morning | daemon not loaded | `sudo launchctl print system/io.github.thorstenpfister.basho.record`; repeat step 5 |
| `not a tournament day` on a tournament day | wrong start date | check `curl -s https://www.sumo-api.com/api/basho/202611`; run once with `--tournament 202611` |
| Every request fails | network, firewall | step 3; `curl -I https://osada.us` on the Mac |
| The run stopped mid-day and did not restart | crash loop, killed | read the log; start it now: `sudo launchctl kickstart -k system/io.github.thorstenpfister.basho.record` (it skips the past ticks) |

## 7. After the tournament

Stop and remove the daemon:

```sh
sudo launchctl bootout system/io.github.thorstenpfister.basho.record
sudo rm /Library/LaunchDaemons/io.github.thorstenpfister.basho.record.plist
```

If you set a wake schedule in step 1, remove it again. Check
`pmset -g sched` first, so you don't cancel a schedule something else on the Mac
needs.

Copy the recordings to the laptop. Keep the full set outside the repository:

```sh
rsync -av <you>@<host>:basho-live/ ~/basho-live-kyushu-2026/
```

Then trim for `testdata/live/`. Every tick folder is self-contained, so you
can just copy the folders you want. For Kyushu 2026 these were kept: the
start, osada's first results (about 15:05), and the end of each division, plus
the playoff on day 15:

```sh
copy() { d=$1; shift; for t in "$@"; do
  mkdir -p testdata/live/$d && cp -R ~/basho-live-kyushu-2026/$d/$t testdata/live/$d/
done; }
copy 20261108 1400 1510 1540 1620 1700 1800
copy 20261112 1400 1510 1550 1700 1750 1810
copy 20261122 1400 1530 1700 1730 1740 1750 1820 1910
```

The recorder does not save sumo-api's banzuke, which the app caches before the
day; `testdata/live/sumoapi/` holds it, fetched once afterwards.

`basho-data record analyze`, the V1 latency comparison, is built after the
tournament, on the full set.
