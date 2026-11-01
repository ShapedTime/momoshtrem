# streamlab

streamlab is the lab for momoshtrem's streaming path: WebDAV → VFS → anacrolix, and HDRezka remote streams.

It runs the real stack in-process, on loopback:
- the anacrolix client with production's config, and the piece cache;
- the torrent service and the activity manager;
- `LibraryFS`, `TorrentFile`, the priority readers and the sessions;
- the WebDAV server.

It runs that stack against synthetic seeders, whose content is a function of the offset so every byte can be checked, and a fake HDRezka CDN. An Infuse-like player drives it.

It answers three questions with data:
- Did a change break something? Correctness, races and leaks.
- Did it make streaming worse? Start, seek, stalls and waste.
- What happened in this stream?

Where to look:
- **When and how to use it:** the `streamlab` skill, `.claude/skills/streamlab/SKILL.md`. Read it before you change the streaming path.
- **Why it's built this way:** `PLAN.md` in this folder. Its decisions are D1 to D10, the suspects F1 to F13 are in §3, and the verdict rules are in §6.
- **The reference:** this file covers every command, the catalogs, the artifacts, and how to extend the lab.

## Contents

- [Quick start](#quick-start)
- [Commands](#commands)
- [Scenarios](#scenarios)
- [Checks](#checks)
- [Metrics](#metrics)
- [Findings](#findings)
- [Known issues](#known-issues)
- [Reports and artifacts](#reports-and-artifacts)
- [Adding a scenario](#adding-a-scenario)
- [Adding a known issue](#adding-a-known-issue)
- [The baseline](#the-baseline)
- [Calibration](#calibration)
- [Limits](#limits)

## Quick start

Run everything from `momoshtrem/`. The lab needs no Postgres and no network.

```bash
cd momoshtrem
go run -tags disable_libutp ./cmd/streamlab list                       # the scenarios and what each guards
go run -tags disable_libutp ./cmd/streamlab explain cold-start         # a scenario, metric, check or known issue
go run -tags disable_libutp ./cmd/streamlab run --scenario steady-play --reps 3
git fetch origin
go run -tags disable_libutp ./cmd/streamlab affected --base origin/main
go run -tags disable_libutp ./cmd/streamlab gate
caffeinate -i go run -tags disable_libutp ./cmd/streamlab ab --base origin/main --reps 5 --scenario steady-play,seek-fwd
go run -tags disable_libutp ./cmd/streamlab check
```

Below, `streamlab …` stands for `go run -tags disable_libutp ./cmd/streamlab …`.
- **`disable_libutp`.** Production builds with `CGO_ENABLED=0`, so it never links libutp. The tag keeps the lab's cgo and `-race` builds on the same code (D3).
- **Prefer `go run` to a binary you built earlier.** `run`, `gate` and `check` run every repetition in a child process of the binary itself, so an old binary tests old code. `ab` and `calibrate` build both sides themselves.

The machine:
- **cgo** is needed for `check` and `ab --race`: the race detector needs it.
- **One heavy run at a time.** Load skews timings, and `compare` calls sides that ran under very different load inconclusive.
- **Long runs** (`ab`, `calibrate`) belong on power, wrapped in `caffeinate -i` on a Mac.
- **The metainfo cache** is `os.UserCacheDir()/momoshtrem-streamlab`.

## Commands

| Command | What it does | Writes | Exit codes | Time on an M1 Pro |
|---|---|---|---|---|
| `run` | Runs scenarios, each repetition in a child process | a run folder | 0 pass, 1 fail, 3 tool error | a gate scenario's rep takes 4–45 s |
| `list` | Lists the scenarios: tier, guards, summary | stdout | 0 | instant |
| `explain <name>` | Describes a scenario (params included), metric, check or known issue | stdout | 0, or 3 for an unknown name | instant |
| `gate` | Runs the gate tier and holds it to the baseline | a run folder, `gate.json`, `gate.md` | 0, 1, 3 | ~20 min |
| `baseline update --reason "…"` | The only way to change the baseline | `internal/streamlab/testdata/baseline.json` | 0, 1 (refused), 3 | a gate run at 2 reps, or none with `--from` |
| `check` | Builds with `-race`, runs the stress tier, then `go test -race` | a run folder, `check.json`, `check.md`, `go-test-race.txt` | 0, 1, 3 | 5–10 min |
| `ab` | An interleaved A/B against a base, confirmed on fresh reps | an ab folder | 0 pass or improvement, 1 fail or regression, 2 inconclusive, 3 tool error | 5–11 min a gate scenario; the whole tier ~2¾ h |
| `compare` | Judges two runs, or an ab folder again | stdout; `--out` writes `compare.json` and `compare.md` | as `ab` | seconds |
| `calibrate` | Measures this machine's noise | `noise.json` in the main checkout's `.streamlab` | 0, 1 (a scenario didn't pass), 3 | 10 reps of every scenario (~5 h for the gate tier) |
| `affected` | Maps a diff to the scenarios that cover it | stdout (`--json`) | 0, 1 with `--strict` and a warning, 3 | under a minute |

`usage` (`streamlab -h`) lists the same commands. PLAN §5 and §8 plan more commands: `record`, `replay`, `capture`, `timeline` and `watch` (S11, S18, S19). None of them exists yet.

### Flags most commands share

| Flag | Default | In | Meaning |
|---|---|---|---|
| `--scenario a,b` | | run, check, ab, calibrate, gate, baseline update | Comma-separated scenarios |
| `--tier t` | `gate` | run, ab, calibrate | A tier's scenarios when no `--scenario` is given. `check` runs the stress tier, and so does `ab --race` |
| `--reps N` | 1; `ab` 5; `calibrate` 10; `baseline update` 2 | all of the above | Measured repetitions per scenario, a side for `ab` |
| `--warmup N` | 1; `check` 0 | all of the above | Repetitions run first and discarded. `check` still counts their races |
| `--set k=v` | | run, check, ab, calibrate | Overrides a scenario param, repeatable; on both sides for `ab`. See `explain <scenario>` |
| `--config prod.yaml` | | run, check, ab, calibrate | A YAML config laid over the defaults. `read_timeout` and `start_paused` are params, set after it |
| `--out dir` | `.streamlab/runs/<id>` | run, check, ab, calibrate, gate | The output folder. `ab` needs a new or empty one |
| `--gomaxprocs N` | 0 (the default) | run, check, ab, calibrate, gate | Pins the children's GOMAXPROCS |
| `--timeout d` | the scenario's | run, check, ab, calibrate, gate | Each repetition's hard timeout: 3 min for most gate scenarios, 4 for dual-reader-close, 6 for stress |

### run

```
streamlab run [--tier gate | --scenario a,b] [--reps 1] [--warmup 1] [--config f] [--set k=v]...
              [--out dir] [--gomaxprocs N] [--timeout d] [--diagnose]
```

- **Without `--scenario`** it runs the gate tier.
- **`--diagnose`** observes in depth:
  - DEBUG records of the prioritizer, the priority readers and the activity manager, in `logs.ndjson`;
  - mutex and block profiles at full rate;
  - the flight recorder all along.

  Its timings aren't faithful, so use it to explain a failure, not to measure.
- **Useful params:**
  - `--set read_timeout=120s` is production's read timeout. The lab's default is 10 s, which bounds what a #195 stall costs.
  - `--set observe=false` runs a repetition unobserved.
  - `--set seed=N` changes the content and the script's random choices.
  - `--set settle=30s` waits longer for quiescence.
- **Exit codes:** 0 when every scenario passes, xfails included; 1 when one fails or a repetition (a warm-up's too) hung or crashed; 3 on a tool error.

### gate

```
streamlab gate [--scenario a,b] [--reps 1] [--warmup 1] [--baseline path] [--out dir] [--gomaxprocs N] [--timeout d]
```

It runs the gate tier, or the gate scenarios you name, and holds them to `internal/streamlab/testdata/baseline.json` ([The baseline](#the-baseline)).

### baseline update

```
streamlab baseline update --reason "why" [--from run-folder] [--scenario a,b] [--reps 2] [--warmup 1] [--baseline path]
```

See [The baseline](#the-baseline).

### check

```
streamlab check [--scenario a,b] [--reps 1] [--warmup 0] [--config f] [--set k=v]... [--out dir]
                [--gomaxprocs N] [--timeout d] [--exe race-binary] [--tests=false]
```

It builds `.streamlab/bin/streamlab-race` with `-race -tags disable_libutp`, unless `--exe` names one. Then it runs the stress tier with it, or the scenarios you name, gate ones included.

**Races.**
- Each repetition's race reports go to `rep-N/race.<pid>` (`GORACE log_path`). `check` parses them into `race_reports`.
- Races are deduplicated by signature: each access's kind, its top frame with file and line, and its top two momoshtrem frames.
- Every scenario gets a `race` check, and any race fails it. A registered race (`known.Races`) explains a failure only by its exact signature. None is registered today.

**Hangs.** A repetition past its hard timeout gets SIGQUIT, and its goroutine dump is kept in `goroutines.txt`.

**`go test -race`.** Then it runs `go test -race -tags disable_libutp` over:
- `./internal/streamlab/...` and `./cmd/streamlab/...`;
- `./internal/vfs`, `./internal/streaming`, `./internal/torrent` and `./internal/sessions`.

`--tests=false` skips it.

**Output.** `check.json` (`streamlab.check/v1`) and `check.md` sum up the build, the verdicts, every race with its scenarios, every hang and the packages.

**Exit codes:** 0 pass, xfails included; 1 for a race, a hang, a failed check or a failed test; 3 for a tool error, such as no cgo.

### ab

```
streamlab ab --base rev [--head rev] [--tier gate | --scenario a,b] [--reps 5] [--warmup 1] [--race]
             [--confirm=false] [--set k=v]... [--config f] [--keep] [--force] [--out dir] [--gomaxprocs N] [--timeout d]
```

Did this change make streaming worse than the base, on this machine, now? PLAN §6 has the rules.

1. **The base.** It's checked out in a worktree, `.streamlab/worktrees/<sha>` in the main checkout, which every worktree of the repository shares. A `flock` on `<sha>.lock` claims it. When another run holds that name, this one uses `<sha>-<pid>`.
2. **The head.** It's your working tree, uncommitted changes included, or `--head rev` in a worktree of its own.
3. **One lab on both sides.** Your `internal/streamlab` and `cmd/streamlab` are copied over the base, and over `--head`'s tree. A base without hooks H1 to H3 is refused: "base predates streamlab hooks".
4. **Builds.** Both sides build side by side. `--race` adds `-race` and the race check, and runs the stress tier unless you name scenarios. Race builds get no timing verdicts (D8).
5. **The screen.** After the warm-ups, the reps run interleaved, each pair led by the other side (AB BA AB …). Each rep's working directory is its side's tree, so its report carries its own commit.
   - The load average is sampled before every rep into `ab.json`, and each child records its own (`env.load`).
6. **The confirmation.** `ab.json` is written, then the comparison. Every scenario whose verdict rests on statistics runs again on fresh reps, as many a side, after their warm-ups, into `confirm/`.
   - `--confirm=false` skips it, and so does `--reps` under 5. Their statistical verdicts stay inconclusive.
7. **Cleanup.** The worktrees and the binaries are removed, unless you pass `--keep`.

Use `--base origin/main` after `git fetch origin`. A local `main` may be stale.

### compare

```
streamlab compare [--force] [--json] [--out dir] <base> <head>
streamlab compare [--json] [--out dir] <ab folder>
```

- **`<base>` and `<head>`** are each a run's folder, its `index.json`, or one `report.json`. With two runs it's a screen: its timing and count changes come out inconclusive.
- **An ab folder** is judged again, its confirmation included. Do that after a `calibrate`, or after the rules change.
- **`--force`** compares runs from two machines, or with different params. Only checks and exact metrics are judged then.

### calibrate

```
streamlab calibrate [--rev main] [--tier gate | --scenario a,b] [--reps 10] [--warmup 1] [--set k=v]... [--config f]
                    [--keep] [--out dir] [--gomaxprocs N] [--timeout d]
```

See [Calibration](#calibration).

### affected

```
streamlab affected [--base main] [--json] [--strict] [files...]
```

It maps changed files to scenarios. The files are `git diff --no-relative` from the merge base with `--base`, uncommitted and new files included. Files you pass stand for the diff, relative to the module (`internal/streaming/prioritizer.go`).

A file maps three ways:
- **By guard (the focus, marked `*`).** A commit in a scenario's `Guards` changed the file (`git show`), or a known issue the scenario guards lives in it. The files of each known issue are listed in `knownFiles` in `internal/streamlab/affected/affected.go`.
- **By package.** A table maps the streaming path's packages to the scenarios that run them:
  - the VFS, WebDAV, sessions, metrics, `common`, config, the library, `go.mod` and `go.sum` map to every scenario;
  - `streaming` and `torrent` map to the torrent scenarios;
  - `hdrezka`, `service/hdrezka*` and `vfs/remote_file.go` map to the HDRezka scenarios;
  - the lab itself maps to every scenario.
- **By linking.** Any other package that `go list -deps ./cmd/streamlab` lists maps to the whole gate tier.

It prints the `ab` lines (the focus first, then every affected gate scenario) and a `check` line for the stress scenarios, all with your `--base`.

It warns when one diff changes code the lab runs and the lab itself (`internal/streamlab/`, `cmd/streamlab/`). With `--strict`, that warning exits 1.

`cmd/momoshtrem/` isn't linked by the lab: its stack (`internal/streamlab/stack`) wires its own copy. So main.go maps only through guards (ffda214 and bf328c5 changed it).

## Scenarios

Generated from the catalog (`streamlab list`). `streamlab explain <name>` prints the full doc, the checks and every param with its default.

| Scenario | Tier | Guards | Own checks | Key metrics | What it does |
|---|---|---|---|---|---|
| `cold-start` | gate | 356e74d, ffda214, F3 | propfind-no-load | time_to_playable, probe_latency, activation_wait, torrents_loaded_by_propfind | A player opens a movie on a cold stack: a Depth 1 PROPFIND of its folder (which loads the torrent today: F3), a stat, a HEAD, the header and footer probed at once on two connections, then play from the start. |
| `steady-play` | gate | 9de9c67, 5355720 | - | time_to_playable, stall_count, seek_updates_per_mib, waste_ratio | A player plays a movie from the start, without seeking. |
| `seek-fwd` | gate | 9de9c67, 5355720 | - | seek_latency_p50, stall_count, waste_ratio, handler_lag_after_abort | Plays from the start, then seeks forward to 40% and 75%. Each seek drops the playing request and opens a new one, as Infuse does. |
| `seek-back` | gate | 9de9c67, 5355720 | - | seek_latency_p50, seek_latency_p95, stall_count, waste_ratio | Plays from 30%, rewinds a few seconds into downloaded data, then seeks back to 5%, where nothing is downloaded. |
| `scrub` | gate | F12 | abort-lag | handler_lag_after_abort, seek_latency_p50, readers_created, read_timeouts | Plays a few seconds, then 10 seeks in 3 s to random positions, each dropped by the next, then plays from where it settled. |
| `probe-burst` | gate | F1, F3 | propfind-no-load, no-sniff-reads | torrents_loaded_by_propfind, sniff_readers, readers_created, residual_download_bytes | Browses a season: a Depth 1 PROPFIND, a HEAD of each episode, then each episode's header and footer probed at once. |
| `parallel-ranges` | gate | efd4c62 | - | time_to_playable, stall_count, probe_latency, readers_created | Two playbacks of one movie at once, at the start and at 60%, while a third connection fetches bounded ranges at 20%, 40%, 80% and 95%. |
| `dual-reader-close` | gate | F6 | close-keeps-windows | pieces_lowered_by_close, stall_time_after, stall_count | Reader A plays from the start. Reader B opens at 60%, reads and pauses, then closes after 31 s while A is paused. |
| `dual-reader-open` | gate | F13 | open-keeps-windows | pieces_lowered_by_open, stall_time_after, stall_count | Reader A plays from 20 MiB before the end and pauses. Reader B then opens a header probe. |
| `pause-resume` | gate | ffda214 | idle-resume | stall_count, stall_time, time_to_playable | Plays, pauses until the activity manager idles the torrent, then resumes on the same request. |
| `reconnect` | gate | 9de9c67, 5355720 | - | stall_count, readers_created, handler_lag_after_abort, leaked_goroutines | The player's connection drops three times while it plays. Each time it reconnects at once with a Range GET from where its buffer ends. |
| `next-episode` | gate | 3907536 | next-prefetch | next_header_prefetched_bytes, next_time_to_playable, time_to_playable, stall_count | Watches the end of episode 1 of a season pack, then opens episode 2 from the start. |
| `small-cache` | gate | F11 | cache-holds-complete | evicted_complete_pieces, seek_latency_p50, read_timeouts | Reads the first 48 MiB of a movie through a 16 MiB piece cache, then seeks back to the start. |
| `etag-stability` | gate | F2 | etag-stable | etag_changes, if_range_honoured | Asks for a file's ETag twice by HEAD and twice by PROPFIND, then resumes with a Range GET whose If-Range is that ETag. |
| `seeder-loss` | gate | - | swarm-recovers | stall_time, rebuffer_ratio, read_timeouts, bytes_downloaded | Plays from a fast seeder behind a proxy and a slow one. The fast one vanishes mid-play and comes back later, and the lab adds it again, as a tracker would. |
| `slow-swarm` | gate | - | swarm-shaped | time_to_playable, stall_count, swarm_rate, read_timeouts | A fresh magnet on a poor swarm: one seeder at 0.8× the bitrate, behind latency and jitter, found after a discovery lag, the info dict fetched from it. Stalls are expected. |
| `corrupt-piece` | gate | #231 | corrupt-caught | integrity_errors, hash_failures, bad_chunks_served, stall_time | Resumes at piece k, which only a corrupt seeder has. It serves k wrong three times, then right, and gets banned; the good seeder stays. |
| `hdrezka-play` | gate | #66 | remote-browse-no-open, remote-one-open-per-get | time_to_playable, probe_latency, hdrezka_upstream_opens, cdn_bodies_left | Opens an HDRezka movie as Infuse does: PROPFINDs, a HEAD, header and footer probes, then 10 s of play. |
| `hdrezka-seek` | gate | #66 | remote-one-open-per-get, abort-lag | seek_latency_p50, hdrezka_upstream_opens, handler_lag_after_abort, cdn_peak_bodies | Plays an HDRezka movie with seeks forward to 40% and 75%, a rewind, and a seek back to 10%. |
| `hdrezka-expiry` | gate | #66 | link-refresh | link_resolves, link_invalidations, seek_latency_p50, hdrezka_upstream_opens | The CDN's links expire (403, 404, then 410) while a player watches. The streamer invalidates, resolves and retries once. |
| `hdrezka-503` | gate | #66 | retry-503 | seek_latency_p50, seek_latency_p95, hdrezka_upstream_opens, stall_count | The CDN answers bursts of 503 while a player watches an HDRezka movie. |
| `hdrezka-stall` | gate | #66, #232 | remote-watchdog, abort-lag | cdn_peak_bodies, handler_lag_after_abort, stall_time, streamer_slots_held | The CDN body stalls, drops and slows, then a scrub runs while every body stalls. |
| `close-blocked-read` | stress | 0d364a2, 46e3423, #225 | ops-bounded | stress_ops, read_timeouts, leaked_goroutines | Reads block on pieces a held seeder doesn't send, and their files close under them, round after round. |
| `seek-pending-read` | stress | 5355720, 0d364a2 | ops-bounded | stress_ops, read_timeouts, seeks_actual | Each worker reads twice while the seeder is held: the first read times out and stays pending, and the second, a seek, must drain it. |
| `abort-blocked-read` | stress | F12 | abort-lag | handler_lag_after_abort, stress_ops, read_timeouts | A client drops a GET while the server's read is blocked, over and over. |
| `remove-pause-idle` | stress | bf328c5, ffda214 | ops-bounded, torrent-consistent | stress_ops, stress_short_reads, read_timeouts | Readers stream one movie while its torrent is removed, paused and idled under them, at random. |
| `remove-while-hashing` | stress | #230, bf328c5 | ops-bounded, torrent-consistent | stress_ops, stress_short_reads | The torrent is removed with its data a random moment into a download, again and again. |
| `concurrent-adds` | stress | bf328c5 | torrent-consistent | stress_ops, stress_short_reads | A season pack's torrent is removed, then added by many callers at once. |
| `many-readers` | stress | efd4c62 | - | stress_ops, probe_latency, readers_created | Many connections, each its own device, read bounded random ranges of one movie at once. |

**The gate tier.** Each scenario takes 15–45 s, with the seeder above the bitrate unless the scenario says otherwise.

**The stress tier.** It's built for `check`:
- a 2 s read timeout;
- an unlimited seeder that the script holds and releases at random moments;
- smaller streaming windows;
- production's concurrent readers of streaming state polling beside the script.

Each strand's choices come from its own generator, so `--set seed=N` replays them. Only the goroutine schedule varies.

**Common params.** Every scenario has these. A scenario can change a default, and `explain` marks those with `*`.

| Param | Default | Meaning |
|---|---|---|
| `seed` | 1 | Seeds the content (so the info hash) and the script's RNG |
| `bitrate` | 1MiB | The player's bitrate, bytes per second |
| `max_buffer` | 16MiB | The player stops reading while its buffer holds this much |
| `playable_bytes` | 0 | Bytes buffered before playback starts; 0 is 2 s of the bitrate |
| `resume_bytes` | 0 | Bytes buffered again that end a stall; 0 is `playable_bytes` |
| `retries` | 2 | How often a playback reconnects after a broken body |
| `seeder_rate` | 3MiB | Each seeder's upload cap, bytes per second; 0 is unlimited |
| `piece_length` | 512KiB | The torrents' piece length |
| `file_size` | 96MiB | The movie's size |
| `stored` | true | The metainfo is in the leecher's store; false fetches the info dict from the seeder |
| `start_paused` | true | `torrent.start_paused`, production's default; set after `--config` |
| `read_timeout` | 10s | `torrent.read_timeout`, set after `--config`. Production's is 120 s |
| `settle` | 15s | How long quiescence waits after the last request |
| `residual_window` | 3s | How long the residual download is measured after quiescence |
| `probe_bytes` | 64KiB | A header or footer probe's size |
| `discovery` | 0s | How long the leecher takes to find the seeders. 0 puts them in the magnet (`x.pe`) |
| `observe` | true | Observe the repetition; false runs it unobserved, to compare timings |
| `watchdog` | 0s | How long without progress trips the watchdog; 0 is half the read timeout |

## Checks

Generated from `scenario.Checks` and the registry. `streamlab explain <check>` prints the same text.

Each check reports a status:
- `pass`, `fail` or `n/a`, where n/a means the rep couldn't tell;
- `xfail`: a known issue's signature explains the failure;
- `xpass`: it passed although a known issue the scenario guards was expected to fail it.

The **correctness checks** are `integrity`, `http`, `race`, `watchdog` and `quiescence`. When one fails in head, the compare fails, whatever base did.

| Check | Scenarios | Known issues that can explain a failure | Passes when |
|---|---|---|---|
| `script` | every scenario | - | The scenario ran its steps to the end: no request it needed failed outright. |
| `integrity` | every scenario | #231 | Every byte received matched the media: zero mismatched bytes. |
| `http` | every scenario | - | Every Range GET answered 206 with the right Content-Range and Content-Length, every HEAD 200 with the size and Accept-Ranges, and no handler panicked. |
| `play-ok` | every scenario | F11, #195 | Every playback and probe got its bytes: no body broke before its end and no request failed. Known signatures, when every failure of the repetition carries the same one: #195, a body silent for half the read timeout or more that broke at an incomplete piece with the seeder connected (and not paused by the lab) and downloads on, on at most two pieces; F11, a body that broke within 3 s of its last byte at a piece anacrolix had complete and the cache no longer holds, or (forward play) at a partial piece whose written chunk there the cache no longer holds and whose read anacrolix logged failing after the completion resync just before (the log capture's proof). A script's Env.Expect excuses the bodies a fault it injects is meant to break, before their first byte at the offset it names, and no others. |
| `quiescence` | every scenario | #225, #230 | Within the settle time after the last request: no request, reader, open file, blocked read or in-flight request left, no goroutine a request leaked, and no session left once the sessions clock is skewed 31 s; with HDRezka, no CDN body open and all 8 streamer slots free. #225 explains a failure where nothing is left but goroutines in anacrolix's waitAvailable under TorrentFile.readContext, after a read timed out. |
| `watchdog` | every scenario | #195 | The watchdog never tripped, and the script ended every fault it declared: while a title's requests were active and a read blocked, with no fault declared for its torrent, something was read or downloaded at least every watchdog param (half the read timeout by default). A trip dumps the labelled goroutines, a flight recorder snapshot, both clients' status and the pieces around each reader. #195 explains a trip that carries its signature, measured at the trip: every stuck piece wanted and incomplete, its torrent active with a seeder connected that the lab didn't pause, nothing progressing for half the read timeout or more, on at most two pieces. A fault still open when the script ends fails the check. n/a with observe=false. |
| `propfind-no-load` | cold-start, probe-burst | F3 | A Depth 1 PROPFIND of a folder loads none of its torrents (F3). n/a when one was loaded before (F3Result.Conclusive). |
| `no-sniff-reads` | probe-burst | F1 | PROPFINDs and HEADs create no reader: nothing sniffs content at offset 0 (F1). Where the MIME table resolves .mkv it passes trivially. |
| `etag-stable` | etag-stability | F2 | HEADs and PROPFINDs of one file give one ETag, and a Range GET with that ETag as If-Range answers 206 (F2). F2's signature: several ETags, and the If-Range GET answered 200. |
| `abort-lag` | scrub, hdrezka-seek, hdrezka-stall, abort-blocked-read | F12, #232 | Every handler ends within abort_lag_max of its client dropping the request (F12). F12's signature: it ended within the read timeout plus a second. An HDRezka file's handler carries #232's: it ended within the remote read timeout plus a second. |
| `close-keeps-windows` | dual-reader-close | F6 | Closing a reader lowers no incomplete piece of another open reader's window (F6; its signature: every lowered piece went to None). n/a when that window had no incomplete wanted piece, or the closing handler didn't return. |
| `open-keeps-windows` | dual-reader-open | F13 | Opening a reader lowers no incomplete piece of another open reader's window (F13; its signature: every lowered piece is in the file's header or footer). n/a when that window had no incomplete piece above High where the new reader prioritizes. |
| `cache-holds-complete` | small-cache | F11 | No piece anacrolix calls complete was evicted from the piece cache (F11). n/a when everything complete fits in the cache. |
| `idle-resume` | pause-resume | #195 | A pause past the idle timeout idles the torrent, and playback resumes after it. n/a when the playback broke before the pause. #195 explains a failure when every body that broke after the resume carries its signature. |
| `next-prefetch` | next-episode | - | Opening an episode prefetches the next one's header at Normal: some of it is complete when the next episode opens (3907536). |
| `swarm-recovers` | seeder-loss | - | The fast seeder the proxy cut off was found again once it came back: the leecher reconnected through the proxy and downloaded from it again, and the playback carried on. |
| `corrupt-caught` | corrupt-piece | - | The corrupt seeder's piece failed the leecher's hash check and came again, anacrolix banned the corrupt seeder alone (its alias), and the good seeder stayed connected. n/a when the corrupt seeder never served its bad chunks. |
| `swarm-shaped` | slow-swarm | - | The swarm was as slow as the scenario says: every byte came through the proxy, its first connection came no sooner than discovery after the first request, and the download ran at most 15% over the seeder's rate. A pacing, proxy or discovery that silently does nothing fails it. |
| `remote-browse-no-open` | hdrezka-play | - | PROPFINDs and HEADs of an HDRezka file make no CDN request: a remote file opens without I/O (#66). |
| `remote-one-open-per-get` | hdrezka-play, hdrezka-seek | - | Each GET of an HDRezka file made one CDN request: its body read straight through, without a reopen (#66). |
| `link-refresh` | hdrezka-expiry | - | Each expiry of the CDN's links cost one Invalidate and one fresh resolve, with no other resolve, and the playback carried on: the streamer refreshes a refused link once (#66). |
| `retry-503` | hdrezka-503 | - | A burst of one 503 is absorbed by the streamer's retry, at a seek and at a mid-stream reopen alike; a burst of two breaks exactly one body (the streamer retries once), which the player's reconnect recovers. |
| `remote-watchdog` | hdrezka-stall | - | A stalled CDN body ends within twice the read timeout plus 2 s (the RemoteFile's idle watchdog closes it once it has had nothing for the read timeout, after the connection's buffers drain) and is reopened where it stopped, and a dropped one is reopened within 2 s: no read breaks. |
| `race` | every scenario `check` and `ab --race` run | - | The repetition's process reported no data race (streamlab check's -race build: the GORACE log of each repetition). A known race explains a failure only when every race of the repetition carries its exact signature: both accesses' kinds, their top frames with file and line, and their top momoshtrem frames. A report cut off before its first access is an unparsed race: a fail. |
| `ops-bounded` | close-blocked-read, seek-pending-read, remove-pause-idle, remove-while-hashing | #230 | Every call the script made directly returned within its bound, plus 5 s of slack. A ReadAt has twice the read timeout: a drain of the pending read, then the read. A Close after its read has one read timeout, since nothing should hold the file. An Open or a RemoveTorrent has the add timeout plus the read timeout, for an add in flight. A call past its bound is stuck: a deadlock the hard timeout would only show as a hung repetition. Its stuck-N.txt holds every goroutine. #230 explains a failure whose stuck calls are all removes and opens, each dump showing its lock cycle. |
| `torrent-consistent` | remove-pause-idle, remove-while-hashing, concurrent-adds | - | Once the script's adds and removes are done, the torrent service and the anacrolix client agree on every library torrent: loaded in both or in neither. A torrent the client still has that the service forgot leaks; one the service lists that the client dropped breaks every open. |

## Metrics

This table is generated from `scenario.Metrics`, which `streamlab explain <metric>` prints ("metric ttfb: timing, in ms, lower is better"). Its thresholds come from `compare.TimingFloor` and `compare.CountFloor` in `internal/streamlab/compare/rules.go`. If the table and the code ever disagree, the code is right: run `explain`.

How compare judges each kind (PLAN §6):
- **Exact metrics are deterministic.** A change on any rep is reported. A worse value is a regression, unless the check that judges it already fails, or is an xfail of an intermittent issue.
- **Counts and timings are judged by their Δ and p.**
  - **Δ** is the Hodges–Lehmann shift: the median of every head − base difference.
  - **p** is from the Mann-Whitney U test. A change past the threshold counts with p < 0.05 and at least 5 reps a side.
  - **Counts** use 3×CV, where the CV comes from `noise.json` for this machine, or 0.10 when it has none.
  - **Timings** get no verdict from a race build.
  - Anything weaker is `inconclusive, add reps`. A single compare is a screen: only `ab`'s confirmation makes a count or timing change a regression or an improvement.
- **A metric whose better is `none`** is only reported as `changed`, never a verdict. `observe_overhead` is the lab's own cost, and it's never judged.
- **Survivor metrics** only exist on the reps that got there: `ttfb`, `time_to_playable`, `next_time_to_playable`, `resume_ttfb`, `seek_latency_p50` and `seek_latency_p95`. When head measured one on fewer reps than base, the result is `inconclusive`.

| Metric | Kind | Unit | Better | Flagged past | What it measures |
|---|---|---|---|---|---|
| `integrity_errors` | exact | bytes | lower | any change | Bytes received that didn't match the media. |
| `panics` | exact | panics | lower | any change | Handler panics net/http recovered. |
| `leaked_goroutines` | exact | goroutines | lower | any change | Goroutines carrying a request's label left after quiescence (not the anacrolix ones a request started, which live with the torrent). |
| `readers_left` | exact | readers | lower | any change | PriorityReaders still registered after quiescence. |
| `requests_left` | exact | requests | lower | any change | Requests whose handler was still running after quiescence. |
| `open_files_left` | exact | files | lower | any change | The open_files gauge after quiescence. |
| `blocked_reads_left` | exact | reads | lower | any change | The blocked_reads gauge after quiescence. |
| `sessions_left` | exact | sessions | lower | any change | Streaming sessions left after the sessions clock was skewed 31 s. |
| `readers_created` | count | readers | none | max(10%, 3×CV, 1) | PriorityReaders created: priority_updates_total{type=initial}, one InitialPrioritize each. A count, not exact: every reconnect after a broken body adds one. |
| `torrents_loaded_by_propfind` | exact | torrents | lower | any change | Torrents a Depth 1 PROPFIND loaded (F3). |
| `sniff_readers` | exact | readers | lower | any change | Readers created by PROPFINDs and HEADs: content sniffed at offset 0 (F1). |
| `etag_changes` | exact | etags | lower | any change | Distinct ETags of one file over HEADs and PROPFINDs, less one (F2). |
| `if_range_honoured` | exact | bool | higher | any change | 1 when a Range GET with the file's ETag as If-Range answered 206 (F2). |
| `reads` | count | reads | none | max(10%, 3×CV, 1) | Streaming reads: reads_total. |
| `bytes_served` | count | bytes | none | max(10%, 3×CV, 1 MiB) | Body bytes the WebDAV server sent for GETs. |
| `bytes_downloaded` | count | bytes | lower | max(10%, 3×CV, 1 MiB) | Data bytes the leecher downloaded from peers. |
| `waste_ratio` | count | ratio | lower | max(10%, 3×CV, 0.05) | bytes_downloaded over bytes_served. |
| `residual_download_bytes` | count | bytes | lower | max(10%, 3×CV, 1 MiB) | Bytes downloaded after the last request closed: through the quiescence wait and residual_window more. |
| `seek_updates_per_mib` | count | updates/MiB | lower | max(10%, 3×CV, 0.5) | priority_updates_total{type=seek} per MiB read through the VFS. |
| `debounced_updates_per_mib` | count | updates/MiB | none | max(10%, 3×CV, 0.5) | priority_updates_total{type=seek_debounced} per MiB read through the VFS. |
| `seeks_actual` | count | seeks | none | max(10%, 3×CV, 1) | Reader seeks that moved: seek_actual_total. |
| `seeks_redundant` | count | seeks | lower | max(10%, 3×CV, 1) | Reader seeks that didn't move: seek_redundant_total. |
| `pieces_downgraded` | count | pieces | none | max(10%, 3×CV, 1) | Completed pieces the prioritizer set back to None: pieces_downgraded_total. |
| `read_timeouts` | count | reads | lower | max(10%, 3×CV, 1) | Server reads that timed out: read_timeouts_total. |
| `slow_reads` | count | reads | lower | max(10%, 3×CV, 1) | Server reads that blocked over 500 ms: slow_reads_total. |
| `over_read_bytes` | count | bytes | lower | max(10%, 3×CV, 1 MiB) | Bytes the players received and never played: their buffers when they seeked or stopped. |
| `pieces_lowered_by_close` | count | pieces | lower | max(10%, 3×CV, 1) | Incomplete pieces of reader A's window whose effective priority dropped when reader B, open 30 s or more, closed (F6). How many are incomplete then depends on the download's progress. |
| `pieces_lowered_by_open` | count | pieces | lower | max(10%, 3×CV, 1) | Incomplete pieces of reader A's window whose effective priority dropped when reader B opened (F13). |
| `evicted_complete_pieces` | count | pieces | lower | max(10%, 3×CV, 1) | Pieces anacrolix believes complete whose data the piece cache (the filecache LRU) has evicted (F11). How many depends on how far the readahead got. |
| `next_header_prefetched_bytes` | count | bytes | higher | max(10%, 3×CV, 1 MiB) | Bytes of the next episode's header complete when it opened: the prefetch at Normal (3907536). |
| `ttfb` | timing | ms | lower | max(15%, 100 ms) | The first playback's time from its GET to its first byte. |
| `time_to_playable` | timing | ms | lower | max(15%, 200 ms) | The first playback's time from its GET to playable_bytes buffered. |
| `seek_latency_p50` | timing | ms | lower | max(15%, 200 ms) | Median time from a seek to playable again. |
| `seek_latency_p95` | timing | ms | lower | max(15%, 300 ms) | 95th percentile time from a seek to playable again. |
| `stall_count` | timing | stalls | lower | max(15%, 1) | Times a player's buffer ran empty while playing. |
| `stall_time` | timing | ms | lower | max(15%, 500 ms) | Time players spent stalled. |
| `rebuffer_ratio` | timing | ratio | lower | max(15%, 0.02) | stall_time over stall_time plus playing time. |
| `stall_time_after` | timing | ms | lower | max(15%, 500 ms) | Reader A's stall time after reader B opened or closed (F6, F13). |
| `next_time_to_playable` | timing | ms | lower | max(15%, 200 ms) | The next episode's time from its GET to playable. |
| `resume_ttfb` | timing | ms | lower | max(15%, 200 ms) | After a pause past the idle timeout, the time from resuming to the next byte. |
| `activation_wait` | timing | ms | lower | max(15%, 100 ms) | Mean time waitForFirstAccess blocked: activation_wait_seconds. |
| `handler_lag_after_abort` | timing | ms | lower | max(15%, 250 ms) | The longest a handler ran after its client dropped the request: server end minus client abort (F12). |
| `propfind_latency` | timing | ms | lower | max(15%, 50 ms) | The slowest PROPFIND. |
| `probe_latency` | timing | ms | lower | max(15%, 100 ms) | The slowest header or footer probe. |
| `wall_time` | timing | ms | none | max(15%, 1000 ms) | The script's wall time. |
| `swarm_rate` | count | B/s | none | max(10%, 3×CV, 64 KiB/s) | What came through the slow seeder's proxy, per second from its first connection to its last byte: the pacing as the leecher saw it. |
| `hash_failures` | count | pieces | lower | max(10%, 3×CV, 1) | Pieces that failed the leecher's hash check: PiecesDirtiedBad. |
| `bad_chunks_served` | count | chunks | none | max(10%, 3×CV, 1) | Chunks the lab's corrupt seeders served wrong. |
| `hdrezka_upstream_opens` | count | requests | lower | max(10%, 3×CV, 1) | Requests the fake CDN got: one per body the streamer opened, retries and refreshes included. A count: under faults, a dropped request may open its body or not. |
| `link_resolves` | exact | resolves | lower | any change | Link calls that found no cached link and resolved one: one per stream, and one after each Invalidate. |
| `link_invalidations` | exact | calls | lower | any change | Invalidate calls: links the CDN refused. |
| `cdn_peak_bodies` | count | bodies | none | max(10%, 3×CV, 1) | The most CDN bodies open at once. The streamer holds at most 8. |
| `cdn_bodies_left` | exact | bodies | lower | any change | CDN bodies still open after quiescence. |
| `streamer_slots_held` | exact | slots | lower | any change | Of the streamer's 8 body slots, those still held after quiescence. |
| `watchdog_trips` | count | trips | lower | max(10%, 3×CV, 1) | Watchdog trips: stretches without progress while requests were active and a read blocked, no fault declared. |
| `benign_storage_errors` | count | reads | lower | max(10%, 3×CV, 1) | anacrolix reads that logged at ERROR ("initial read failed") and then succeeded on a retry: #219's benign filecache noise. |
| `storage_read_failures` | count | reads | lower | max(10%, 3×CV, 1) | anacrolix reads that gave up after resyncing the piece's completion ("read failed after completion resync"): data the piece cache lost (F11). |
| `log_errors` | count | records | lower | max(10%, 3×CV, 1) | ERROR log records, momoshtrem's and anacrolix's. |
| `log_warnings` | count | records | lower | max(10%, 3×CV, 1) | WARN log records, momoshtrem's and anacrolix's. |
| `observe_overhead` | timing | % | lower | never judged (the lab's cost) | The observer's samplers, piece mirror and log capture: their busy time over the process's CPU time. The budget is 2%. The mutex and block profiles' cost inside the runtime isn't in it: an A/B against observe=false measures that. |
| `race_reports` | exact | races | lower | any change | Distinct data races the repetition's process reported, by signature (streamlab check's race build only). |
| `stress_ops` | count | ops | none | max(10%, 3×CV, 1) | Operations the stress script completed, whatever their end: reads, requests, adds, removes, pauses, listings. |
| `stress_short_reads` | count | reads | none | max(10%, 3×CV, 1) | Stress reads that ended before their range's end, as a read timeout, an abort or a removed torrent ends them: expected under stress. |
| `op_latency_max` | timing | ms | lower | max(15%, 500 ms) | The slowest direct VFS call of a stress script (ops-bounded). |

`report.md` also shows things no metric judges:
- the piece timeline: Now→complete p50 and p95, and the reader's lead;
- the mutex delay under the anacrolix client lock, and under piece `SetPriority` calls (F7's cost);
- the samplers' cost.

## Findings

The observer (S5) turns what it recorded into hints. Each points at its evidence (a file in the repetition's folder, or a request ID), and none changes a verdict. They're listed in `report.md`, under "Findings" and in each rep's "Observation".

| Rule | Means |
|---|---|
| `stall-below-now` | During a stall, the reader's piece was incomplete and below Now with downloads on: the prioritizer's window didn't cover the playhead |
| `starved` | The swarm sent less than the bitrate over the stall: expected |
| `stall-unexplained` | The swarm sent more than the bitrate and the piece was at Now: the delay is on our side (requests, the read path), or several readers shared the swarm |
| `competing-windows` | Readers of one file farther apart than the streaming window: their windows compete |
| `handler-lag` (F12) | A handler outlived its client's drop by more than `abort_lag_max` |
| `sniff-at-zero` (F1) | A reader was prioritized at offset 0 though no open request asked for it. Needs `--diagnose`, since it reads the prioritizer's DEBUG records |
| `watchdog` | A trip, pointing at its dumps; it carries #195 when the trip has #195's signature |
| `storage-loss` (F11) | anacrolix gave up reading data it believed it had, after resyncing the piece's completion |

## Known issues

The registry is `internal/streamlab/known` (D6). A check that fails because of a registered bug reports `xfail` and doesn't block. An issue explains a failure only when the failure carries its **signature**, measured when it happened.

There are two kinds of issue:
- **Deterministic** issues fire on every repetition while they exist. They explain failures only in the scenarios that guard them (`Guards`). Anywhere else they're a plain `fail`. In a scenario that guards one, a pass is an `xpass`: the bug appears fixed.
- **Intermittent** issues (#195, #225) explain failures only at their rate: at most 25% of the reps, and never all of them, counted per scenario and over the run. A firing in a scenario with fewer than 2 measured reps gets 2 confirmation reps. Over the rate, it's a `fail`.

| ID | Tracked in | Kind | Checks it can explain | What |
|---|---|---|---|---|
| F1 | #171 (PLAN §3 F1) | deterministic | no-sniff-reads | Without a MIME type for .mkv, every GET, HEAD and PROPFIND sniffs 512 bytes at offset 0 (only where the MIME table doesn't resolve .mkv) |
| F2 | #73 | deterministic | etag-stable | The ETag changes with every request, so If-Range never matches |
| F3 | #83 | deterministic | propfind-no-load | PROPFIND loads the torrent of every file it lists |
| F6 | #216 | deterministic | close-keeps-windows | Closing a reader open 30 s or more resets the whole file's priorities, other readers' windows too |
| F11 | #215 | deterministic | cache-holds-complete, play-ok | Reading a piece the filecache LRU evicted breaks the stream |
| F12 | #217 | deterministic | abort-lag | A handler blocked in a read outlives its client's abort |
| F13 | #209 | deterministic | open-keeps-windows | A new reader's InitialPrioritize overwrites other open readers' windows |
| #195 | #195 | intermittent, at most 25% of reps | play-ok, idle-resume, watchdog | anacrolix loses a writer wakeup: a read waits up to the 1-minute keep-alive |
| #231 | #231 | deterministic | integrity | The responsive reader serves a chunk before its piece's hash check |
| #232 | #232 | deterministic | abort-lag | A dropped HDRezka request's handler holds its streamer slot until its read gives up |
| #225 | #225 | intermittent, at most 25% of reps | quiescence | A timed-out read's goroutine outlives its closed TorrentFile |
| #230 | #230 | deterministic | ops-bounded, quiescence | Removing a torrent with its data while a piece hashes deadlocks it until restart |

No data race is registered (`known.Races` is empty).

Failures the registry doesn't explain yet:
- **#266.** A body sometimes breaks right after its first chunk, at an incomplete piece: `16384 bytes, then: unexpected EOF (no known signature: silent 0s, …)`. It shows up mostly in `parallel-ranges`, on about 1 rep in 75. `compare` judges such a check by its reps, but `run` and `gate` fail on it.
- **#268.** A head that can't start the stack reads as a tool error (exit 3), not a fail.

## Reports and artifacts

```
<checkout>/.streamlab/                     gitignored, with a .gitignore of "*" in it too
  runs/
    latest -> <id>                         the last run, gate or check written without --out
    <date>-<time>-<pid>/                   a run, gate or check
      index.json  index.md                 one row per scenario (streamlab.index/v1)
      gate.json   gate.md                  gate only: the run held to the baseline (streamlab.gate/v1)
      check.json  check.md                 check only: the build, verdicts, races, hangs, go test (streamlab.check/v1)
      go-test-race.txt                     check only: go test -race's output
      <scenario>/
        report.json  report.md             the scenario over its reps (streamlab.report/v1)
        warmup-1/  rep-1/ … rep-N/         one child process each; confirmation reps follow the measured ones
          rep.json  child.log              the child's result, and its stdout and stderr
          samples.ndjson                   every sampler's readings
          timeline.ndjson                  piece arrivals against the readers' positions
          timeline-<title>.svg             the same, plotted
          logs.ndjson                      the captured slog records (DEBUG ones with --diagnose)
          mutex.pprof  block.pprof         for go tool pprof
          watchdog-N-goroutines.txt        a trip: the labelled goroutine profile
          watchdog-N-status.txt            … both clients' status
          watchdog-N-pieces.txt            … the pieces around each reader
          watchdog-N-trace.out             … the flight recorder (go tool trace)
          failure-N.txt                    both clients' status at a timed-out read
          goroutines.txt                   a rep past its hard timeout (SIGQUIT)
          stuck-N.txt                      stress: every goroutine when a call outlived its bound
          race.<pid>                       check and ab --race: the GORACE log
    <date>-<time>-<pid>-ab/                ab, or the folder you gave --out
      ab.json                              the builds, commits, schedule and the load before each rep (streamlab.ab/v1)
      base/  head/                         each a run folder as above: the screen
      confirm/base/  confirm/head/         the confirmation's fresh reps
      compare.json  compare.md             the verdict (streamlab.compare/v1)
    <date>-<time>-<pid>-calibrate/
  bin/                                     streamlab-race (check); ab-<pid>/ (ab's builds, removed after)

<main checkout>/.streamlab/                shared by every worktree of the repository
  worktrees/<sha>  worktrees/<sha>.lock    ab's and calibrate's checkouts, claimed by flock; <sha>-<pid> when taken
  noise.json                               calibrate's CVs, per machine (streamlab.noise/v1)
```

- **The main checkout** is the parent of `git rev-parse --git-common-dir` when that's the checkout's own `.git`. A bare repository's worktree or a submodule keeps its own. In the main checkout itself, both trees above are one folder.
- **Runs in an agent's worktree** (`.claude/worktrees/…`) live in that worktree's `.streamlab` and go with it. The worktrees `ab` checks out and `noise.json` are shared, so a calibration serves every worktree.

**Reading `report.md`.** From top to bottom:
1. The verdict and why: the failing checks, the broken reps with their dumps, the expected failures, any xpass.
2. The scenario's summary, guards, run, tool (commit) and environment (CPU, load, the MIME table).
3. **Checks:** the status, the known issue, the status on each rep, and the evidence.
4. Races and findings.
5. **Metrics:** the median, the p95 and every rep's value.
6. **Repetitions.**
7. **Sessions**, for the last good rep: each device's player stats, and each request with its range, status, TTFB, bytes and lag after an abort. Request IDs (`s1.2`) match the `slreq` label on goroutines.
8. The torrent's waste and residual download, and the resources.
9. **Observation:** the samplers' cost, the piece timeline, contention, logs, faults, watchdog trips and findings.
10. The params and the artifacts.

**Reading `compare.md`.** From top to bottom:
1. Its first line is the verdict, worst first: `fail` > `regression` > `inconclusive` > `improvement` > `pass`.
2. A table of the two sides: commits, reps, load per CPU, machine, race build.
3. Whether timings and counts were judged, the count noise (defaults or calibrated) and the confirmed scenarios.
4. A row per scenario.
5. A section for each scenario that changed:
   - the checks that moved;
   - each flagged metric: base and head medians with their n, the Δ, p, the threshold, the status and a note;
   - after a confirmation, the status reads `regression (screen regression)`, and the note gives both runs' one-sided p.
6. The intermittent issues, with their firings on each side and Fisher's p.

## Adding a scenario

1. **Write it.** Scenarios live in `internal/streamlab/scenario`:
   - `gate.go` has the everyday gate scenarios, `faults.go` the swarm-fault ones and `hdrezka.go` the HDRezka ones;
   - the stress tier is in `stress_reads.go` and `stress_torrents.go`.

   A scenario is a `*Scenario{Name, Tier, Doc, Guards, Params, Media, Swarm, Stack, Script, Checks, Key, Timeout}`. The first paragraph of `Doc` is its summary, which `list` shows. The rest says what it guards and why.
2. **Register it.**
   - The gate.go scenarios are listed in `catalog` in `catalog.go`. The other files append theirs in an `init`.
   - `scenario.Validate`, which the package's tests run, checks the names, docs, tiers, declared checks, key metrics and param defaults.
3. **Script it through `Env`.**
   - Drive the stack with `e.Device`, `e.Conn`, `e.Player(dev, t).Open/Seek`, `p.Watch`, `p.Stop`, `e.Probe`, `e.Head`, `e.Propfind` and `e.Go`. Read params from `e.P`.
   - Return an error only when a step couldn't run: the `script` check. A playback that breaks isn't an error, since `play-ok` judges it.
   - A fault the script injects declares itself with `e.Fault(name, title)` and ends it, so the watchdog doesn't trip on it. A body the fault is meant to break is excused with `e.Expect(title, offset, n, why)`. A fault still open at the end fails `watchdog`.
4. **Record its own checks and metrics.**
   - Record a check with `e.Check(id, status, known, evidence)`, and declare it as a `CheckDef` in `Checks`, with its ID in the scenario's `Checks`.
   - Record a metric with `e.Metric(name, v)`, and declare it as a `report.MetricDef` in `Metrics`, with a kind, a unit and a better direction.
   - Choose the kind honestly. `exact` is only for what's deterministic, because any change of it is a regression.
5. **Set its `Guards`.** These are the commits (7 or more hex digits) and known-issue IDs (F-numbers, `#n`) the scenario was written to protect. They do three things:
   - **`affected`** makes the scenario a focus for a changed file when a guarded commit touched that file, or when a guarded known issue lives in it. For a new issue, list its files in `knownFiles` in `internal/streamlab/affected/affected.go`. A guard that isn't a commit in this history maps no files.
   - **Known issues:** a deterministic issue explains a failure only in a scenario that guards it, and only there does a pass become an `xpass`.
   - **Reports:** `list`, `explain` and `report.md` show them.

   If the scenario exercises a production package the table in `affected.go` doesn't list, add a row there. Otherwise a change to that package maps to the whole gate tier.
6. **Prove it.**
   - Run `go test ./internal/streamlab/scenario/`, then `streamlab run --scenario <name> --reps 5`, and check it passes on main.
   - Show it catches what it guards: reintroduce the bug on a scratch branch, never pushed. `ab` must give `fail` or `regression` there, and an unchanged tree must not (D7).
7. **Record its baseline** (gate scenarios). Until then, the gate notes "not in the baseline". Run `streamlab baseline update --scenario <name> --reason "add <name> (#n)"`.
8. **Document it.** Add a row to the scenario table here, and to PLAN §5's tier table.

A new scenario is a lab change. It lands in its own PR, not beside a streaming change (the skill's anti-gaming rule).

## Adding a known issue

Only a bug the lab has shown gets an entry. It first gets its own issue, labelled `streaming` and linked from #171 (PLAN §3).

1. **Find the signature.** It's what, measured at the failure, proves this bug and nothing else. Examples:
   - #195: a silence of half the read timeout or more, at most two wanted and incomplete pieces, a seeder connected that the lab didn't hold, downloads on.
   - #230: one mutex in the goroutine dump, `MarkNotComplete` waiting on its write lock and the hasher's `NewReader` on its read lock.

   A different failure of the same check must stay a plain `fail`. A loose signature turns a regression into an xfail.
2. **Write the matcher** next to the code that measures the check. It sets the outcome's `Known` to the issue's ID. The existing ones:
   - `playKnown` (play-ok) and `orphanedReads` (#225 on quiescence) in `scenario/collect.go`;
   - `lagKnown` (abort-lag: F12 and #232) in `scenario/hdrezka.go`;
   - `stuckKnown` (ops-bounded) and `quietKnown` (quiescence) in `scenario/stress_deadlock.go`;
   - the watchdog's trip in `observe/watchdog.go`.
3. **Register it.** Add an `Issue{ID, Title, Ref, Checks, MaxRate, Doc, AppliesTo}` to `known.Issues`, or in its own `init` as `known/stress.go` does.
   - `MaxRate: known.Always` makes it deterministic. 0 makes it intermittent at 25%.
   - `AppliesTo` limits where it can happen. F1 needs a MIME table without `.mkv`.
   - A race goes in `known.Races`, with the exact signature that `check.md` printed.
4. **Guard it.** Add the ID to the `Guards` of the scenarios that reproduce it, and its files to `knownFiles` in `affected.go`.
5. **Prove the signature is narrow.** This step is required.
   - **A table test of near misses.** Cases that look like the bug but aren't must not match. For example:
     - `TestPlayKnown`: stalls on three pieces, or more stalls than read timeouts;
     - `TestLagKnown`: a lag past the timeout, or a handler still running;
     - `TestStuckKnown`: a stuck read beside the remove;
     - `TestHeldSeederStallIsNotKnown`: a stall the lab caused by holding the seeder.
   - **A mutation test.** On a scratch branch, never pushed, break the production code another way that fails the same check. The scenario must report a plain `fail`, not `xfail`. Then drop each condition from the matcher in turn: a test must fail each time. Record both in the PR.
6. **Record the expectation:** `streamlab baseline update --scenario <its scenarios> --reason "register #n as a known issue"`. The scenario's expectation becomes `xfail:<ID>`.
7. **Explain it.** `streamlab explain <ID>` prints the entry. Add it to the table above.

**When it's fixed,** the fix's PR removes the entry and updates the baseline, with the reason. The skill's "Known flakes and known issues" section has the steps.

## The baseline

`internal/streamlab/testdata/baseline.json` (`streamlab.baseline/v1`) is the gate's expectations. For each gate scenario it holds:
- each check's expected status: `pass`, `n/a` or `xfail:<issue>`;
- each exact metric that every rep measured with one value.

It never holds a timing (D5). An intermittent issue that happened to fire while it was recorded isn't an expectation. It was recorded on macOS, where the MIME table resolves `.mkv`.

**`streamlab gate` fails when:**
- the run fails;
- an exact metric moves or wasn't measured;
- a check leaves its expected status, or doesn't run: a scenario's checks are fixed, so a missing one means the lab changed under the baseline;
- an `xfail` turns `xpass`. The bug appears fixed, so the registry and the baseline must say so.

**It only notes:**
- #195 firing within its rate;
- a check that was `n/a` this time. F3, F6, F11, F12, F13 and #232 have n/a paths that depend on timing.
- F1's expectations on a host whose MIME table differs from the recording host's;
- a scenario that isn't in the baseline.

**Changing it.** Only `baseline update --reason` changes the file:

```bash
streamlab gate                                                     # what moved, in gate.md
streamlab baseline update --reason "why" [--scenario a,b]          # runs the gate itself (2 reps)
streamlab baseline update --reason "why" --from ../.streamlab/runs/<id>   # or takes a gate run's reports
git diff internal/streamlab/testdata/baseline.json
```

- **What it writes.** It prints what changed, and appends the reason, the commit and the date to the file's `updates`. A digest covers the history and the expectations.
- **What it refuses:**
  - a failing run (exit 1);
  - a run with an `xpass` (exit 1): remove the issue from the registry first.
- **Hand edits.** The gate refuses a file edited by hand, because the digest won't match. An update of an edited file must re-record every scenario in it, so a hand edit can't hide under a fresh digest.
- **The rule.** A baseline change in the same PR as a streaming change needs a written reason (the skill's anti-gaming rule).

## Calibration

`streamlab calibrate --rev origin/main --reps 10` runs the revision, with the working tree's lab, in a worktree like `ab`'s base. It stores each scenario's metric CVs in the main checkout's `.streamlab/noise.json`, under this machine's key: host, OS and architecture, CPU model and count. It also records the commit, the date, the reps and the load.
- **How compare uses it.** `compare` and `ab` read it for count thresholds (3×CV). Without it, they assume a CV of 0.10, and `compare.md` says so.
- **Skipped scenarios.** A scenario that didn't pass is skipped, because its numbers would measure the failure. Then calibrate exits 1, or 3 when the tool couldn't run one.
- **Rejudging.** `streamlab compare <ab folder>` judges an earlier A/B again with the new calibration.
- **When.** A calibration must postdate S6 (#177), which changed how seeders pace their uploads: numbers from before it don't compare with later runs.

The gate tier hasn't been calibrated on any machine yet: at 10 reps it takes about 5 hours.

## Limits

- **The lab isn't the device.**
  - Loopback has no RTT, and the swarm is one or two seeders. The lab runs TCP only, while production also runs uTP.
  - The read timeout is 10 s (production's is 120 s; use `--set read_timeout=120s`).
  - It's a regression detector, not a predictor: test changes on the Apple TV too.
- **Timings are noisy and bimodal (#233).** Most `ab` verdicts on small changes are `inconclusive`. Only large effects reach `regression`.
- **What it can't see today:**
  - F4 and F5 need hook H4 (#183).
  - F7 has no check.
  - F10: nothing makes `detectFormat` block.
  - F1 can't show on macOS.
  - A 9de9c67 revert, and 356e74d as reverted (#265).
  - The wiring in `cmd/momoshtrem/main.go`: the lab has its own copy.
  - Downloads beside a stream. `internal/downloads` and other linked packages map to the gate tier in `affected`, but the stack never runs the downloads manager. So Normal-priority downloads running beside a stream aren't covered.
- **Known rough edges:** #266 (a flake with no signature) and #268 (a head that can't start reads as exit 3).

The lab's own tests:

```bash
go test ./internal/streamlab/... ./cmd/streamlab/...
go test -race -tags disable_libutp ./internal/streamlab/... ./cmd/streamlab/...
STREAMLAB=1 go test ./internal/streamlab/...    # the slow ones: transfers, an end-to-end ab, the 3,000-run null simulation
```
