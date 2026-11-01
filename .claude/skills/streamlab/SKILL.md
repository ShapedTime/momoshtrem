---
name: streamlab
description: Guards momoshtrem's fragile streaming path with the streamlab lab. Use it before you change, review or merge anything under momoshtrem/internal/vfs, internal/webdav, internal/streaming, internal/torrent, internal/sessions or internal/hdrezka, internal/service/hdrezka_source.go, or the streaming wiring in cmd/momoshtrem/main.go (torrent client, activity manager, VFS, WebDAV server), the torrent and streaming settings in internal/config (readahead, urgent buffer, read timeout), a bump of anacrolix/torrent or golang.org/x/net in go.mod. Also use it whenever someone reports buffering, a slow start, a slow seek, a stall or a stuck read in Infuse or on the Apple TV. It covers the workflow (affected, gate, ab, check), how to read verdicts and exit codes, which scenario guards each fragile point, and debugging playbooks.
---

# streamlab

The lab runs the real streaming stack in-process against synthetic seeders and an Infuse-like player. It judges your change against `main`. This file is the procedure.
- Every command, the metric and check catalogs, and the artifacts: `docs/streamlab/README.md`.
- The rules and their reasons: `docs/streamlab/PLAN.md` (§6 has the verdict rules).

## Before you start

- **The command.** `streamlab …` below means `go run -tags disable_libutp ./cmd/streamlab …`, run from `momoshtrem/`. `go run` rebuilds from your tree every time. `run` and `gate` test the binary they run in, so a binary built before your last edit tests old code.
- **The base.** Run `git fetch origin`, then pass `--base origin/main`. Local `main` is often stale: in an agent worktree it's checked out elsewhere. `affected` and `ab` use whatever base you give them.
- **One heavy run on the machine at a time.**
  - Never run two `ab` at once, and never run `ab`, `gate` or `check` beside builds, Playwright suites or docker.
  - Check first with `pgrep -fl streamlab`. If a run is going, wait or ask the user.
  - Why: when the two sides ran under very different load, compare calls their timings inconclusive, and load makes #195 fire.
- **Long runs.**
  - Keep the Mac on power with the lid open, and wrap the command in `caffeinate -i`. A run on battery was killed by idle sleep.
  - Start it in the background (Bash `run_in_background`) with its output in a log, and wait for it to exit. Don't poll it.
  - If the harness refuses a compound command line, put the steps in a small script and run that.
- **Where runs land.** They go in your checkout's `.streamlab/runs/`. In a worktree, that folder goes when the worktree does, so paste the verdict into the PR first.
- **When the lab isn't needed.** Changes to `_test.go` files or docs only don't need it: nothing the lab runs changed.

## Workflow

| # | Step | Command | Time on an M1 Pro | Needs |
|---|---|---|---|---|
| 1 | Which scenarios cover the diff | `streamlab affected --base origin/main --strict` | under a minute | exit 0 (1 is the anti-gaming warning, below) |
| 2 | Correctness against the baseline | `caffeinate -i streamlab gate` | ~20 min | PASS |
| 3 | Is it worse than main? | `caffeinate -i streamlab ab --base origin/main --reps 5 --scenario <affected's list> --out ../.streamlab/ab-<issue>-1` (a new folder each time) | 5–11 min a scenario | read it (below) |
| 4 | Races, deadlocks and leaks under `-race` | `caffeinate -i streamlab check --scenario <affected's stress list>` | 5–10 min | PASS |
| 5 | Read the results | `compare.md` first, then `report.md` of each scenario that didn't pass; `gate.md`, `check.md` | | |
| 6 | Paste the verdict into the PR | the template below | | |

1. **`affected`** lists the scenarios that run your code. A `*` marks those written for it: they guard a commit that touched your file, or a known issue that lives there. It ends with ready-made `ab` and `check` lines that use your `--base`.
   - Its widest `ab` line covers every affected gate scenario: run it before asking for review. Its first line, the `*` scenarios only, is for quick loops while you work.
   - "No scenario covers them" means the lab doesn't run your code. Say so in the PR, and test on the device.
   - `cmd/momoshtrem/main.go` isn't linked by the lab: it wires its own copy of the stack in `internal/streamlab/stack`. `affected` maps main.go only through the guards of ffda214 and bf328c5. A wiring change is invisible to the lab until the stack mirrors it, and mirroring it is a lab change (see the anti-gaming rule).
2. **`gate`** runs the 22 gate scenarios once each, after a warm-up, on your tree.
   - It holds their checks and exact metrics to `internal/streamlab/testdata/baseline.json`. It never judges timings.
   - A FAIL names the scenario and the broken expectation in `gate.md`. Fix it before you spend hours on `ab`.
   - A one-rep play-ok failure that looks like #266: see "Known flakes".
3. **`ab`** does this:
   - checks `origin/main` out in a worktree and copies your lab over it, so both sides run the same lab;
   - builds both sides and runs their reps interleaved (AB BA AB …);
   - re-runs on fresh reps every scenario whose verdict rests on statistics: the confirmation.

   Measured: the whole gate tier took 2 h 42 min to 2 h 49 min, and five scenarios took 55 min.
4. **`check`** builds the lab with `-race` and runs the stress scenarios, then `go test -race` over the lab and the streaming packages. `--tests=false` skips `go test`.

**For the PR:**

```markdown
### streamlab
- `affected --base origin/main`: <n> scenarios, focus <names>; no warning
- `gate`: **PASS** (xfails as in the baseline)
- `ab --base origin/main` (<sha>) `--reps 5 --scenario <list>`: **<VERDICT>** (exit <n>)
  > <compare.md's first line>
  - <scenario>: <metric> <base> → <head> (Δ, screen p, fresh p) …
  - changed, with no better direction: <scenario>: <metric> <base> → <head> (p) …, or "none"
  - load per CPU, base / head: <from compare.md>; count noise: <defaults or calibrated>
- `check --scenario <list>`: **PASS**, 0 races
- What the lab can't see here, and how it was tested on the device: <…>
```

## Reading verdicts

Verdicts, worst first: `fail` > `regression` > `inconclusive` > `improvement` > `pass`. The worst scenario's verdict is the run's. The first line of `compare.md` is the verdict and its reason.

- **`inconclusive` is the normal result for an unchanged tree.**
  - Lab timings are bimodal: a cold start connects at once or about a second later (#233). Five reps a side rarely settle a timing.
  - Six unchanged-tree runs of the gate tier all came out inconclusive, with 13 to 18 of 22 scenarios passing, and never a regression.
  - Only large effects reach `regression`. 356e74d's old behaviour moved `activation_wait` from 33 ms to 3334 ms.
- **`inconclusive` with p > 0.2 on every flagged metric means "no detectable change at 5 reps".** Write exactly that. It's not a pass, and it's not a regression.
  - This applies only when every inconclusive row is a metric whose note says "past the threshold but not significant" or "didn't hold up on fresh reps". p is the p column of `compare.md`.
  - These are never "no detectable change":
    - a check that failed on some head reps ("a flake or a regression? add reps");
    - an intermittent issue over its rate ("over its known rate, but not significantly more than base");
    - a metric head didn't reach, or reached on fewer reps ("head never got there?", "the rest never got there");
    - a metric with too few reps on a side ("needs 5 reps a side");
    - a correctness check that was n/a in head ("couldn't tell in head");
    - skewed load ("the sides ran under very different load").

    Report them as they are, with their evidence.
- **A single compare is only a screen.**
  - `compare base head`, `ab --confirm=false` and an ab folder without `confirm/` turn every timing and count regression into `inconclusive` ("a single compare is a screen: confirm with ab").
  - Only the confirmation in `ab` makes a timing or count change a verdict. It needs:
    - the screen at one-sided p ≤ 2/252;
    - the fresh reps at p ≤ 1/252;
    - both runs together past the threshold.
- **What makes a `fail`, and none of it is noise:**
  - A correctness check that fails in head fails the compare, whatever base did. The correctness checks are `integrity`, `http`, `race`, `watchdog` and `quiescence`.
  - A rep that hung or crashed fails it too.
  - Any other check fails when it fails on every head rep, or on significantly more reps than in base (Fisher's test).
  - **The exception** is a check that failed only because an intermittent known issue fired over its rate: #195 on `watchdog` or `play-ok`, #225 on `quiescence`. That issue's firings are judged against base's. They're a `fail` when the issue fired on every head rep, or over its rate and significantly more often than base. Otherwise they're `inconclusive` ("over its known rate, but not significantly more than base (load?): add reps"). Skewed load never excuses a firing on a correctness check.
- **A check that failed on 1 of 10 head reps and none of base's** is `inconclusive` ("a flake or a regression? add reps"). If it has #266's exact shape ("Known flakes" below), say so. Any other shape may be a regression: add reps, and treat a second firing as yours.
- **Known issues.** `xfail → xpass` is an improvement ("known issue X appears fixed"). `pass → xfail` of a deterministic issue is a regression ("known issue X is back").
- **Thresholds.** Both kinds need 5 reps a side and Mann-Whitney p < 0.05.
  - Timings count past max(15%, a floor): 100 ms for TTFB, 200 ms for time to playable, 1 stall, and so on.
  - Counts count past max(10%, 3×CV, a floor). A machine without `calibrate` uses a CV of 0.10, and `compare.md` says which.
  - Metrics with no better direction (`pieces_downgraded`, `reads`, `wall_time` …) are only reported as `changed`, even under a `pass` or an `inconclusive`. Report every `changed` row in the PR: a 9de9c67-style revert shows only as `pieces_downgraded` changed.
  - All of them: README, "Metrics".

**Exit codes of `compare` and `ab`:**

| Exit | Verdict | What to do |
|---|---|---|
| 0 | pass or improvement | Paste it. For an xpass, see "Known flakes and known issues" below. |
| 1 | fail or regression | Read the scenario's section in `compare.md`. A fail names the check, its reps and its evidence. A regression names the metric, Δ and both runs' p. Debug with the playbooks below. A confirmed regression has already replicated on fresh reps: don't rerun it away. |
| 2 | inconclusive | If every inconclusive row is a metric with p > 0.2 (the rule above), report "no detectable change at 5 reps". If something flagged and didn't hold up, add reps (below). Report anything else as it is. |
| 3 | tool or environment error | `ab`'s own errors are on stderr. "base predates streamlab hooks": the base is older than #173, so use a newer one. "isn't empty": give `--out` a new folder. A build error: your head doesn't build. A side that couldn't run a scenario shows in `compare.md`, not on stderr, as an "**Error:** head couldn't run it: …" line in the scenario's section. Open that rep's `child.log` (`head/<scenario>/rep-N/child.log` in the ab folder). Your change may stop the stack from starting (#268), so don't just retry. |

`run`, `gate` and `check` exit 0 on a pass (xfails included), 1 on a fail and 3 on a tool error. A 3 from `gate` or `run` on a scenario main runs can be #268: your change may stop the stack from starting. Read the rep's `child.log` (`.streamlab/runs/<id>/<scenario>/rep-N/child.log`), and don't just retry.

**The PR's verdict comes from default flags only.** Use the `--scenario` list from `affected`, and `--reps 10` when you add reps. Runs with `--set`, `--config`, `--timeout`, `--force` or `--confirm=false` are for debugging, never the PR's verdict. Reviewers run `affected --strict` themselves.

**Add reps** means one more `ab`:
- over the inconclusive scenarios only, with `--reps 10`, the same base and an idle machine. It takes twice as long per scenario.
- Stop after that run. If it's still inconclusive, report the flagged metrics with their Δ and p values, and leave the call to the reviewer and the device.
- Don't loop until it says pass, don't pick the run you like, and don't touch thresholds.

## Anti-gaming rule

- **Not in the same PR as a streaming change**, unless the PR gives a written reason a reviewer can check: edits to the baseline, the thresholds or floors, the registry or its signatures, the scenarios, the checks, or any other judgement code. All of `internal/streamlab/` and `cmd/streamlab/` counts. `ab` runs one lab on both sides, so a looser lab hides a regression from it.
- **`streamlab affected --strict` exits 1 on such a diff.** Paste its warning and your reason. By default, lab changes land in their own PR first.
- **`baseline.json` changes only through `streamlab baseline update --reason "…"`.** It records the reason, the commit and the date, and the gate refuses a file edited by hand (a digest).

## Safety first (D1) and bugs the lab finds (§3)

- **D1.** Lab and hook PRs change no production behaviour. A streamtrace hook PR passes PLAN §7 R5's A/B at off, summary and detailed, with exact metrics identical.
- **§3.** A bug the lab finds gets its own issue, labelled `streaming` and linked from #171, with the evidence and the command that reproduces it. Don't fix it inside a lab PR, and don't widen a signature to make it pass.

## Fragile points and what guards them

The mapping comes from each scenario's `Guards` (`streamlab list`) and from the registry (`internal/streamlab/known`). `streamlab explain <name>` describes any scenario, check, metric or known issue.

| Commit | What it fixed | Guarded by | Can the lab see a revert? |
|---|---|---|---|
| efd4c62 | Removed the shared reader cache: one PriorityReader per connection | parallel-ranges; many-readers (stress) | by guard; never mutation-tested |
| 9de9c67 | Incomplete pieces behind the cursor are no longer downgraded (stalls) | steady-play, seek-fwd, seek-back, reconnect | **no**: a revert is inconclusive. Only `pieces_downgraded` moves, and it has no direction (#265) |
| 5355720 | Priority updates decoupled from the anacrolix reader's seek (a deadlock) | seek-pending-read (stress); steady-play, seek-fwd, seek-back, reconnect | by guard (`ops-bounded`, hung reps); never mutation-tested |
| 46e3423 | PriorityReader.Read no longer holds its mutex across the blocking read (Close deadlocked) | close-blocked-read (stress) | by guard; never mutation-tested |
| 0d364a2 | readContext gets a private buffer per read, and drains a pending read (a race and a leak) | close-blocked-read, seek-pending-read (stress) | yes: `ab --race` fails it on `integrity` and `race` |
| 356e74d | Piece priorities set before the activation wait (cold start) | cold-start | partly: the old behaviour, rebuilt by hand, is a confirmed regression; the plain revert is inconclusive (#265) |
| ffda214 | The activation wait under `start_paused` | cold-start, pause-resume (`idle-resume`), remove-pause-idle (stress) | by guard; never mutation-tested |
| 7b2d9c8 | The reader's readahead set to the urgent window; readahead 64 → 32 MB | no scenario names it | only as timings and counts in an `ab`: ttfb, time_to_playable, stall_*, waste_ratio, bytes_downloaded |
| 0e834fc | The reader's readahead back to urgent + readahead; no priority reset for readers open under 30 s (probe thrash) | no scenario names it; its 30 s rule is what dual-reader-close (F6) exercises | only as timings and counts: probe_latency, readers_created in cold-start and probe-burst |

Also guarded: 3907536 (the next episode's prefetch) by next-episode, and bf328c5 (the serialised torrent lifecycle) by remove-pause-idle, remove-while-hashing and concurrent-adds.

| Suspect (PLAN §3) | Guarded by | Check; metrics | What the lab shows |
|---|---|---|---|
| F1: content sniffed at offset 0 (no `.mkv` MIME type) | probe-burst | `no-sniff-reads`; sniff_readers; the `sniff-at-zero` finding (needs `--diagnose`) | **not on macOS**: its MIME table resolves `.mkv`, so the check passes trivially |
| F2: a new ETag on every request (#73) | etag-stability | `etag-stable`; etag_changes, if_range_honoured | xfail |
| F3: PROPFIND loads torrents (#83) | cold-start, probe-burst | `propfind-no-load`; torrents_loaded_by_propfind | xfail |
| F4: OpenContext holds `fs.mu` across GetOrAddTorrent | nothing | | **no**: it needs H4 (#183) |
| F5: InvalidateTree leaves `tree` nil | nothing | | **no**: it needs H4 (#183) |
| F6: a close resets other readers' windows (#216) | dual-reader-close | `close-keeps-windows`; pieces_lowered_by_close, stall_time_after | xfail (n/a on some reps) |
| F7: the prioritizer runs on every 32 KiB read | nothing | seek_updates_per_mib, debounced_updates_per_mib; report.md's "Contention" line for SetPriority; mutex.pprof | counts only, no check |
| F10: `detectFormat` has no timeout | nothing | | **no**: nothing makes it block. If one did, quiescence would catch the goroutine: it carries the request's label |
| F11: the LRU evicts complete pieces (#215) | small-cache | `cache-holds-complete`, `play-ok` (F11's signature); evicted_complete_pieces, storage_read_failures; the `storage-loss` finding | xfail |
| F12: a handler outlives its client's abort (#217) | scrub; abort-blocked-read (stress) | `abort-lag`; handler_lag_after_abort; the `handler-lag` finding | xfail |
| F13: a new reader overwrites others' windows (#209) | dual-reader-open | `open-keeps-windows`; pieces_lowered_by_open | xfail |

The registry also holds:
- #195: intermittent; it explains `play-ok`, `idle-resume` and `watchdog`.
- #225: intermittent; it explains `quiescence`; close-blocked-read guards it.
- #230: remove-while-hashing.
- #231: corrupt-piece, `integrity`.
- #232: hdrezka-stall, `abort-lag`.

## Debugging playbooks

**Start small:** one scenario, a few reps, with `streamlab run --scenario <name> --reps 3`.

**`--diagnose`** shows why something happened:
- the DEBUG records of the prioritizer, the priority readers and the activity manager, in `logs.ndjson`;
- full-rate mutex and block profiles;
- the flight recorder, all along.

Its timings aren't faithful: diagnose with it, measure without it. The artifacts are in `.streamlab/runs/latest/<scenario>/rep-N/`.

**A stall** (a player runs dry, a read blocks, the watchdog trips):
1. Reproduce it:
   - steady-play should never stall;
   - seek-fwd covers stalls after a seek, pause-resume stalls after a wake;
   - in slow-swarm and seeder-loss, stalls are expected.
2. Read the findings in `report.md` first:
   - `stall-below-now`: the playhead's piece was below Now, so the prioritizer's window missed it. Look in `prioritizer.go` and `reader.go`.
   - `starved`: the swarm sent less than the bitrate. Expected.
   - `stall-unexplained`: the swarm was fast and the piece was at Now. The delay is in our read path.
   - `competing-windows`: readers of one file are farther apart than the window.
3. Then the timeline: `timeline-<title>.svg` and `timeline.ndjson` show piece arrivals against the reader. `samples.ndjson` has the samplers' readings.
4. After a watchdog trip, read its dumps:
   - `watchdog-N-status.txt`: both clients' status (interested, choked, requests).
   - `watchdog-N-goroutines.txt`: labelled; `slreq` names the request.
   - `watchdog-N-pieces.txt`: the pieces around each reader.
   - `watchdog-N-trace.out`: open it with `go tool trace`.

   `failure-N.txt` has both clients' status at each timed-out read.
5. #195's signature is anacrolix's lost wakeup, not your bug: at most 2 wanted, incomplete pieces, a seeder connected, and nothing arriving for half the read timeout. More pieces, or a seeder the lab held, is yours.

**A slow start or a slow seek:**
1. Reproduce it:
   - cold-start: activation_wait, probe_latency, time_to_playable;
   - seek-fwd and seek-back: seek_latency_p50 and p95;
   - also scrub, next-episode and hdrezka-seek.
2. One run shows the shape, not a verdict. Timings need `ab`.
3. Read the "Piece timeline" in `report.md`: Now→complete p50 and p95, how often the reader sat at an incomplete piece, and its lead.
4. Today the urgent window's pieces arrive in random order, so the first byte waits for most of the window (#233). That's the baseline, not your regression.
5. The `handler-lag` finding (F12) means a dropped request's handler still held its Now window while the new position waited.

**A leak** (`quiescence` fails, or a `*_left` metric or leaked_goroutines is above 0):
1. Reproduce it with reconnect, scrub, parallel-ranges, or close-blocked-read (stress).
2. The evidence of `quiescence` says what's left.
3. A leaked goroutine's stack carries `# labels: {"slreq":"s1.2"}`. Match that ID to the Request column of the Sessions table in `report.md` to find the request that left it.
4. Goroutines left only in anacrolix's `waitAvailable` under `TorrentFile.readContext`, after a read timeout, are #225: known and intermittent.
5. `--set settle=30s` tells a slow teardown from a leak.

**A race or a deadlock:**
1. Run `streamlab check --scenario <name> --reps 3`.
   - `check.md` lists each race by signature, with its scenarios. `check.json` has `race_reports`.
   - Each rep's raw race log is `rep-N/race.<pid>`.
2. A hang leaves the rep's `goroutines.txt`: the runner sends SIGQUIT at the hard timeout. An `ops-bounded` failure leaves `stuck-N.txt`, with every goroutine at the stuck call.
3. `--set seed=N` replays a stress script's choices. Only the goroutine schedule varies.
4. Did main have it too? Run `streamlab ab --race --base origin/main --scenario <name> --reps 2 --warmup 0` (about 5 min for two scenarios).
5. No race is registered as known (`known.Races` is empty), so any race fails.

## Known flakes and known issues

- **#266 has no signature yet.**
  - What it looks like: `play-ok` fails on one rep with `<N> bytes, then: unexpected EOF (no known signature: silent 0s, piece K (piece incomplete, seeder connected, downloads on, piece not in the cache))`. Most often it's parallel-ranges' `infuse-ranges` request, at piece 182, after 16384 bytes.
  - In `ab` it reads as inconclusive. In `gate` or `run`, the scenario fails.
  - What to do: rerun that scenario alone (`streamlab gate --scenario <name> --reps 3`). If it happened once and has that shape, it's #266: add the evidence to #266 and say so in the PR. If it happens again, treat it as yours.
- **#195** marks each broken body with "(known stall (#195): …)". It's an xfail only when:
  - every broken body of the rep carries the signature, on at most 2 pieces;
  - it fires on at most 25% of the reps, and never all of them.

  A firing in a scenario run with fewer than 2 reps gets 2 confirmation reps. Over its rate it's a fail. Load makes it fire more often.
- **xfail and xpass.**
  - A deterministic known issue (the F-numbers, #230 to #232) fails its check on every rep while it exists.
  - In a scenario that guards it, that's an `xfail`. Anywhere else it's a plain `fail`.
  - When a fix makes the check pass, it becomes an `xpass`:
    - `ab` calls it an improvement;
    - `gate` fails with "appears fixed … remove it from the registry";
    - `baseline update` refuses the run until the entry is gone.
- **When your fix flips one to xpass,** make these changes in the same PR as the fix:
  1. Run `ab` first, so it shows `xfail → xpass`.
  2. Delete the issue's entry from `internal/streamlab/known`. The scenario keeps its Guard: it now guards the fix.
  3. Run `streamlab baseline update --scenario <its scenarios> --reason "F12 fixed (#217): abort-lag xfail:F12 → pass"`.
  4. Run `streamlab gate`: it passes again. Explain all of this in the PR body. It's the usual exception to the anti-gaming rule, and it needs that reason.


## Production, read-only (placeholder for S19, #190)

The production tools (`capture`, `timeline`, `watch`, `replay`) aren't built yet; #190 fills this section in. The rules hold already (PLAN D9 and §8):
- Only GETs, only to `127.0.0.1`, only through the SSH tunnel the user opens to the loopback metrics port.
- No `/api` mutations, restarts, config changes or deploys. Turning metrics or tracing on in the VM's `.env` is the user's job.
- Never send a request to production's WebDAV: a GET there loads and downloads a torrent (§8: `run --target` is for local compose only).
- The repo is public: no hostnames, addresses or secrets in issues, PRs or files.
