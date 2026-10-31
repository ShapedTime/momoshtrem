# streamlab: profile, debug and guard the streaming path

> Status: agreed plan · 2026-10-16 · Epic #171
> A separate track from v2 and ops. Issues are labelled `streaming`.

## 0. How to use this plan

- The work is tracked in issues #172–#190 under epic #171, using the normal workflow in `CLAUDE.md`: claim, branch `<issue#>-<slug>`, and a PR with `Closes #n`.
- This plan fixes **decisions and boundaries**, not implementations. Each issue's agent designs its approach within them.
- If an implementation has to break a decision here, update this file in the same PR and explain why on the issue.
- **The safety rule (§2 D1) is not negotiable.** Until the lab exists, nothing in this track changes how production streams.
- **Bugs found by the lab are not fixed in the lab's PR.** Each one gets its own `streaming` issue, linked from #171.

## 1. Goals and non-goals

The streaming path is WebDAV → VFS → anacrolix, plus HDRezka remote streams. It took months of on-device tuning against Infuse, and git history shows how fragile it is:
- **Readahead values flip-flopped** between 8, 32, 40 and 64 MB (7b2d9c8, 0e834fc, 69f8d25).
- **A shared reader cache was added, then removed** (18fdec7, efd4c62).
- **Read-path deadlocks and leaks** were fixed in 0d364a2, 46e3423 and 5355720.
- **Cold-start races** were fixed in 8c5c2da, ffda214 and 356e74d.

Each of these was verified by hand on the Apple TV.

**Goals.** Agents get one tool that answers three questions with data:
1. **Did my change break something?** Correctness, races and leaks.
2. **Did it make streaming worse?** Start, seek, stalls and waste.
3. **What is happening in this stream?** Locally, or read-only in production.

**Non-goals:**
- Fixing the suspected bugs in §3. The tool demonstrates them, and each fix is its own issue.
- Continuous production monitoring and alerting: no Prometheus/Grafana server, no pager.
- Any production mutation from the tool.

## 2. Decisions

| # | Decision |
|---|---|
| D1 | **Safety net first.** Phase 1 changes no production behaviour. S1 touches no production file, and S2 adds only additive hooks (§4). The streamtrace hooks (Phase 3) come after the lab, and the lab must show they change nothing (§7 R5). |
| D2 | **Real code, synthetic world.** The lab runs the real stack in-process against a synthetic seeder. Content is a pure function f(seed, offset), so every byte served is checked. |
| D3 | **The lab mirrors production's client config.** It uses `torrent.ClientConfigFor` (H2), with overrides only for loopback, port, DHT, trackers, uTP and logger. Never `atorrent.TestingConfig`: it sets `MaxAllocPeerRequestDataPerConn=5` and `KeepAliveTimeout=1ms`. Production is `CGO_ENABLED=0`, so it runs anacrolix's pure-Go uTP and never links libutp. `-tags disable_libutp` keeps the lab's cgo and `-race` builds on that same code. Over loopback the lab still sets `DisableUTP` and runs TCP only. That is a known difference from production, which the realism tier covers. |
| D4 | **One subprocess per scenario repetition.** slog's default logger, the MIME table, the `sync.Pool`s and `DefaultServeMux` are process-wide. A subprocess gives a clean goroutine baseline and contains deadlocks: on a hard timeout the runner sends SIGQUIT and keeps the goroutine dump. |
| D5 | **Counts are the regression signal, timings are judged only by A/B.** Exact and count metrics are deterministic and can be gated. Timing metrics depend on the machine: they get a verdict only from an interleaved A/B on one machine, with repetitions and a significance test. No timing baseline is committed. |
| D6 | **Known issues are expected failures.** A check that fails because of a known bug (§3) reports `xfail` and doesn't block. When a fix lands, it flips to `xpass` and the report says so. The registry is `internal/streamlab/known`. A known issue explains a failure only when the failure carries its **signature**. #195: a body silent for half the read timeout or more, at an incomplete piece with the seeder connected and downloads on. F12: a lag that ends within the read timeout. Each suspect has its own, and a repetition whose failures carry several, each its own, names them all. **Deterministic suspects** (F2, F12, …) fire on every repetition by nature. They explain failures, and turn into `xpass`, only in the scenarios that guard them. **Intermittent issues** (#195; #225, a timed-out read's goroutine left behind) explain failures only at their rate. Such an issue is a **fail** when it fires on every repetition, or on two or more and more than its rate (25% by default). That's counted over the scenario's repetitions and over the whole run's (the run tally). One firing among several repetitions can't tell a regression from the known rate, so the runner **confirms** it: when an intermittent issue fires in a scenario with fewer than 2 measured repetitions, the runner runs 2 more and judges over all of them. A stray #195 (1 of 3) stays `xfail`; a regression fires again and fails. |
| D7 | **The tool must catch the bugs we already had.** Scratch branches that revert 356e74d (cold start), 9de9c67 (stalls) and 0d364a2 (goroutine leak) must each be flagged by `ab`. |
| D8 | **Race detection is a separate pass.** `streamlab check` builds the lab with `-race` and runs a seeded stress tier. Race builds never produce timing verdicts. |
| D9 | **Production is read-only.** Agents only issue GETs to `127.0.0.1` through the user's SSH tunnel to the loopback metrics port (ops PLAN D8). They never call `/api` mutations, restart anything, change config or deploy. Turning tracing on in the VM's `.env` is the user's job. |
| D10 | **The interface is a Go CLI plus a skill.** `cmd/streamlab` writes JSON (schema-versioned) and Markdown reports. `.claude/skills/streamlab/SKILL.md` tells agents when and how to use it. |

## 3. Suspected live bugs the tool must surface

Most of these come from reading the code during planning; F2 and F3 were already open issues (#73, #83). None is fixed by this track. The lab confirms or rules out each one, and a confirmed bug gets its own issue.

| ID | Suspect | Where |
|---|---|---|
| F1 | The `alpine:3` image has no `/etc/mime.types`, and Go's built-in table has no `.mkv`. So `http.ServeContent` sniffs 512 B at offset 0 and seeks back on every GET and HEAD, which sets Now priority on the header pieces. PROPFIND's `getcontenttype` does the same per file. Tests on macOS/Ubuntu can't see it, because those hosts have a MIME table. | `webdav/server.go`, x/net `prop.go:403` |
| F2 | ModTime is `time.Now()` in every `Stat`. x/net's default ETag (ModTime + size, `prop.go:450`) therefore changes with each request, so `If-Range` never matches and the server answers 200 with the whole file (#73). For GET and HEAD the ETag comes from the opened file's `Stat`, so fixing `LibraryFS.Stat` alone isn't enough. `webdavFile.ETag` and `webdavFile.ContentType` are dead code: x/net checks the `os.FileInfo`. | `vfs/library_fs.go:Stat`, `vfs/torrent_file.go:229`, `vfs/remote_file.go:74`, `webdav/server.go:254,286` |
| F3 | PROPFIND opens every listed file, and for a torrent that runs `GetOrAddTorrent` (#83). | x/net `prop.go:170`, `vfs/library_fs.go:openTorrentFile` |
| F4 | `OpenContext` holds `fs.mu.RLock` across `GetOrAddTorrent`, which can wait up to `add_timeout`. A pending writer (`InvalidateTree`) then blocks every new reader. | `vfs/library_fs.go:267` |
| F5 | `InvalidateTree` sets `tree=nil` before rebuilding. `OpenContext`, `Stat` and `ReadDir` then dereference `fs.tree.pathMap` with no nil check. | `vfs/library_fs.go:672` |
| F6 | Closing a reader that was open ≥30 s resets the whole file's priorities, including the windows of other readers still open. | `streaming/reader.go:Close` |
| F7 | The prioritizer runs on every 32 KiB read. Each non-debounced update loops `SetPriority` over ~(urgent+readahead)/piece pieces, and every call takes the anacrolix client lock. | `vfs/torrent_file.go:ReadAt`, `streaming/prioritizer.go` |
| F10 | `detectFormat` opens a second reader with no timeout. | `streaming/reader.go:263` |
| F11 | The filecache LRU can evict pieces anacrolix still believes are complete. The Bolt piece-completion store is created but unused. | `torrent/client.go:InitStorage` |
| F12 | `seekAndReadAtLeast` (the WebDAV `ReadAt` path) and `readWithTimeout` (`Read`) read under `context.Background()` + `read_timeout`, not the request context. If a client aborts while a read is blocked (an Infuse seek), the handler, its `PriorityReader` and its Now window live on for up to 120 s, competing with the new position. | `vfs/torrent_file.go:364,288` |
| F13 | `NewPriorityReader` runs `InitialPrioritize` for every new reader. That sets the header and footer pieces to explicit High file-wide, which overwrites another open reader's Now/Readahead window. It's F6's mirror image, on open instead of close (#209). | `streaming/reader.go:98`, `streaming/prioritizer.go:InitialPrioritize` |

(F8 and F9 from planning were folded into F12 and the HDRezka scenarios.)

The lab found two more (S6, #177):
- **#231.** The responsive reader serves a chunk before its piece's hash check, so a bad peer's bytes reach the player.
- **#232.** A dropped HDRezka request's handler holds its streamer slot until its stalled read times out. It's F12's HDRezka counterpart.

## 4. Production-code hooks

All are additive or pure refactors.

| Hook | Change | Slice |
|---|---|---|
| H1 | `vfs.NewStaticFS([]StaticMovie, []StaticEpisode) *LibraryFS` in `internal/vfs/static_fs.go`, moved out of `export_test.go`. `export_test.go` keeps `NewTestFS`/`TestMovie`/`TestEpisode` as aliases. | #173 |
| H2 | `torrent.ClientConfigFor(cfg, cc) *atorrent.ClientConfig`. `NewClient` delegates to it, and a parity test covers it. | #173 |
| H3 | `ActivityManager.SetCheckInterval(d)`, set before `Start`. It is a fixed 30 s today. | #173 |
| H4 | A `build func() *DirectoryTree` field on `LibraryFS`: `buildTreeFromDB` by default, a static builder from `NewStaticFS`. `rebuildTree` calls `fs.build()`. This lets the lab invalidate the tree, which F4 and F5 need. | #183 |
| H5 | `InitStorage` returns the vendored `*piececache.Cache` (`internal/torrent/piececache`, missinggo's filecache with pins) instead of missinggo's `filecache`. It takes the same parameters, so the lab's `InitStorage(dir, cfg.Torrent.GlobalCacheSize)` still compiles. Without pins it creates the same files and makes the same evictions as upstream; a differential test proves it. With pins, pinned pieces sit outside the LRU and a streaming reserve is never pinned. | #201 (#77 P3) |

The leecher finds the seeder through the real `service.add()` path: the magnet carries `x.pe=<seeder addr>`, which anacrolix maps to `PeerAddrs` (`spec.go:42`).

Cold start is faithful. After `add()` sets every piece to None, anacrolix dials no peers until the first read raises priorities. The activation wait therefore measures real dial and handshake time when the info dict comes from the `MetainfoStore`. With metadata from peers, the connection that fetched it stays open.

## 5. The lab

```
momoshtrem/cmd/streamlab/              CLI: run, list, explain, gate, check, ab, compare, calibrate,
                                       affected, record, replay, capture, timeline, watch
momoshtrem/internal/streamlab/
  media/     f(seed, offset) content (splitmix64), MKV / MP4 (moov-at-end) overlays, multi-file layouts,
             metainfo builder + cache (os.UserCacheDir()/momoshtrem-streamlab)
  swarm/     generated-storage seeders (paced reads), TCP proxy (latency, jitter, bandwidth, drop,
             vanish), discovery delay, corrupt piece (dialled by alias), partial seeders, metadata-from-peers
  cdn/       fake HDRezka CDN (httptest + ServeContent over media; 403/404/410 expiry, 503 bursts,
             slow/drop/stall), fake hdrezka.LinkResolver and slot probe, wired through the real Streamer
             and NewHDRezkaSource
  stack/     the system under test, wired as cmd/momoshtrem/main.go does, on 127.0.0.1:0 with no server
             timeouts; lab middleware: request ID, pprof labels, server-side TTFB/status/bytes/abort
  client/    WebDAV primitives (Propfind, Head, Probe, Play, Seek, Pause, Reconnect, Parallel), one
             Transport per connection, one User-Agent per simulated device
  player/    buffer model: TTFB, time to playable, seek latency, stalls, rebuffer ratio, integrity
  scenario/  Go-defined scenarios {Name, Tier, Doc, Media, Swarm, Stack, Script, Checks, Guards},
             --set params, a seeded RNG; Run runs one repetition in-process; the gate catalog
  observe/   samplers, piece-arrival timeline, slog capture (--diagnose), mutex/block profiles, watchdog
  checks/    integrity, HTTP, quiescence, priority invariants
  known/     the known-issue registry (D6): signatures, rates, xfail/xpass
  report/    streamlab.report/v1 JSON + report.md; streamlab.compare/v1
  runner/    subprocess per rep, warm-up, ABAB interleave, pinned GOMAXPROCS, timeouts, .streamlab/
  capture/   prod pull, timeline, replay format
  testdata/baseline.json   exact and correctness expectations only
```

**Seeder storage.**
- A custom `storage.ClientImpl` whose `PieceImpl.ReadAt` generates bytes from `media`. `WriteAt` is a no-op and `Completion()` is always complete, so no gigabytes are written.
- The seeder, the integrity check and the fake CDN all read from `media.Layout.ReadAt`.
- **Upload rate.** A seeder's upload rate paces its storage reads, not anacrolix's `UploadRateLimiter`. The limiter's retry timer is one of #195's lost writer wakeups (signature D). Real peers don't share that bug, so the lab shouldn't add it.
- **Shared IP.** anacrolix bans peers by IP, and every lab peer is 127.0.0.1. A seeder that serves bad data is therefore dialled by its alias, `0.0.0.0:port`, which connects to the same host. anacrolix then sees an IP of its own, so a ban drops that seeder alone, as production drops one bad peer.

**Leecher storage.** The leecher uses the real `torrent.InitStorage`, the filecache LRU that production uses (after H5, `piececache` with no pins).

**Stack.** It is wired exactly as `cmd/momoshtrem/main.go` does:
1. `InitStorage`, then the leecher.
2. `NewActivityManager` → `NewMetainfoStore` → `NewService(WithMetainfoStore, WithPieceStorage, WithSpeedSampler)`.
3. `NewStaticFS` → `SetTorrentService`, `SetReaderTracker`, `SetSessions(NewRegistry(WithClock))`, `SetMetrics(metrics.New(perRunRegistry))`, `SetRemoteSource` (it comes with S6, #177).
4. `webdav.NewServer(...)`, `SetMetrics` (the same metrics; without it, `requests_inflight` stays 0 and the quiescence check passes trivially), then `.Handler()`.

The config is `config.DefaultConfig()` with an optional `--config prod.yaml` overlaid, then the lab's params. `read_timeout` (10 s; production 120 s) and `start_paused` override `--config`: use `--set read_timeout=120s` for production's. A fingerprint of the effective config goes into every report, and `report.md` shows the read timeout.

**Middleware.** The lab's middleware runs each request under `pprof.Do(labels{"slreq": id})`. Goroutines spawned by `readContext` and `detectFormat` inherit the label, so a leaked goroutine names the request it came from.

### Tiers

**Gate tier.** Each scenario takes 15–45 s. The seeder is rate-limited above the bitrate unless the scenario says otherwise.

| Scenario | Guards |
|---|---|
| `cold-start`: PROPFIND, HEAD, concurrent header and footer probes, then play | 356e74d, ffda214 |
| `steady-play`, `seek-fwd`, `seek-back`, `reconnect` | 9de9c67, 5355720 |
| `scrub`: 10 seeks in 3 s | F12 |
| `probe-burst`: season PROPFIND and sniffs | F1, F3 |
| `parallel-ranges` | efd4c62 |
| `dual-reader-close` | F6 |
| `dual-reader-open`: A's window, then a probe opens | F13 |
| `pause-resume` across the idle threshold | ffda214 |
| `next-episode` | 3907536 |
| `slow-swarm` (stalls expected), `seeder-loss` | |
| `corrupt-piece`: a partial good seeder, and one that serves piece *k* wrong | #231 |
| `small-cache` | F11 |
| `etag-stability` | F2 |
| `hdrezka-{play,seek,expiry,503,stall}` | #66; `stall` also #232 |

**Stress tier** (`check`, `-race`, seeded):
- close during a blocked read (`close-blocked-read`)
- seek during a pending read (`seek-pending-read`)
- remove, pause or idle during a read (`remove-pause-idle`)
- remove with its data during a piece hash (#230, `remove-while-hashing`)
- concurrent adds of one hash (`concurrent-adds`)
- many readers of one file (`many-readers`)
- a client abort during a blocked read (F12, `abort-blocked-read`)
- `InvalidateTree` during opens (after H4: F4, F5; S12 adds it to the tier)

The stress scenarios run production's concurrent readers of streaming state beside their scripts: the reader tracker, the service's lists and details, the activity manager, the sessions, the tree and a metrics scrape. They hold the seeder with `Seeder.HoldUploads`, so reads block, and release it at random moments. They use a 2 s read timeout, an unlimited seeder and smaller streaming windows (the `window` param: 4 MiB readahead, a quarter of that urgent). The code paths are production's; only the sizes differ.

Each strand of a script's choices draws from its own generator, derived from the seed, the scenario and the strand: the pollers, the chaos loop, the script itself, and each worker in each round. So `--set seed=N` replays the offsets, sizes and release moments. Only the goroutines' schedule varies.

**Realism tier:** Sintel or Big Buck Bunny in-process with real DHT and trackers. Its reports use the same schema and never feed the gate.

### Checks

| Check | Passes when |
|---|---|
| Integrity | Zero mismatched bytes. |
| HTTP | A Range GET answers 206 with the right `Content-Range` and `Content-Length`, and no handler panics. |
| Play-ok | Every playback and probe gets its bytes: no body breaks and no request fails. Besides the known issues (D6), one more excuse exists, for faults the lab injects on purpose: a script's `Env.Expect(title, offset, n, why)`. It excuses at most *n* bodies of that title that break before their first byte at that offset while it's open. Only `hdrezka-503`'s burst of two 503s uses it. |
| Quiescence | After the last request, within the settle time: `ReaderTracker.Count()==0`; the `open_files`, `blocked_reads` and `requests_inflight` gauges are 0; no `slreq`-labelled goroutine running momoshtrem code, or started by momoshtrem on a library function (anacrolix-rooted ones a request started, such as peer connections and hashers, are reported as inherited); `sessions.Count()==0` after skewing the session clock 31 s; zero open CDN bodies, and all 8 streamer slots free (a probe through the real `Streamer` that reaches no CDN). This is the leak signal that matters, because production never tears down. |
| Priority invariants | Stated over the **effective** priority of incomplete pieces while downloads are allowed: `Piece.State()` can't read the explicit priority. During playback, `[pos, pos+urgent)` is Now. `residual_download_bytes` measures what downloads after the last close. |

**Watchdog.** If requests are active, no fault is running, and the client has made no progress for N s, it dumps:
- the labelled goroutine profile
- a `runtime/trace` `FlightRecorder` snapshot
- `client.WriteStatus`
- the piece runs

A trip fails the `watchdog` check (`--set watchdog=<d>`, half the read timeout by default). #195 explains it only with #195's signature measured at the trip: every stuck piece wanted and incomplete, a seeder connected that the lab didn't pause, downloads on, nothing progressing for half the read timeout or more (whatever the `watchdog` param), and at most two pieces.

A fault a scenario injects on purpose declares itself, for its torrent or for all (`Env.Fault(name, title)`). The watchdog doesn't trip for a torrent while it has one. A fault still open when the script ends fails the check: a forgotten fault would hide a hang.

**Findings.** Rules that turn data into hints, each pointing at evidence. For example, "stall while the playhead's piece was below Now" points to the prioritizer, and "handler lived N s after the client aborted" is F12.

**What observation can see.** anacrolix v1.60.0 publishes a piece state change only when a piece's completion, progress or pending state changes, not when its priority moves between wanted levels (Readahead to Now). So the piece timeline is exact for arrival, and the priority under each reader is read from anacrolix (`PieceState`) by the readers sampler. Measure mode stays at INFO and samples lightly: each sampler reports its cost, the budget is 2% of the process's CPU, and `--set observe=false` runs a repetition unobserved for comparison. `--diagnose` adds the prioritizer's, priority readers' and activity manager's DEBUG records, full-rate profiles, and the flight recorder all along.

## 6. Reports and verdicts

**Layout.** A run writes `.streamlab/runs/<date>-<time>-<pid>/` at the repo root (or `--out`): `index.json` (`streamlab.index/v1`) and `index.md`, one row per scenario; per scenario, `report.json` and `report.md`, and a folder per repetition (`warmup-1/`, `rep-1/`, …, and the confirmation reps after the measured ones) with the child's `rep.json`, its `child.log`, `goroutines.txt` after a hard timeout, and both clients' status at every timed-out read (`failure-N.txt`). Each child gets a temp folder of its own (removed after it) and a stdin pipe its parent holds: if the parent dies, the child dumps its goroutines and exits. `run` exits 0 when every scenario passes (xfail included), 1 when one fails or a repetition (a warm-up's included) timed out or crashed, 3 on a tool or environment error.

**`check`** (S7) builds `go build -race -tags disable_libutp` of `cmd/streamlab` into `.streamlab/bin/streamlab-race` and runs the stress tier with it.
- **Race logs.** Each repetition gets `GORACE="log_path=<rep dir>/race halt_on_error=0 exitcode=0"`. The `WARNING: DATA RACE` blocks become `race_reports` on the repetition and on the report.
- **Deduplication.** Races are deduplicated by their signature. It holds each access's kind, its top frame with file and line, and its top two production momoshtrem frames by function name, the lab's own left out. A report cut off before its first access is kept as an unparsed race, so it fails.
- **The race check.** check adds a `race` check to every scenario it runs. Any race fails it. A known race explains a failure only by its exact signature (`known.Races`).
- **Then `go test -race`** over the lab and the streaming path's packages.
- **The summary.** It writes `check.json` (`streamlab.check/v1`) and `check.md` beside the run's index: the build, the verdicts, every race with its scenarios, every hung repetition with its goroutine dump, and `go test`'s packages.
- **Exit codes.** 0 pass (xfail included), 1 fail (a race, a hang, a failed check or test), 3 tool error (no cgo, say).

**`report.json`** (`streamlab.report/v1`) is one scenario over its repetitions. It contains:
- **tool:** version and git (commit, dirty, branch)
- **env:** Go version, cgo, race, GOMAXPROCS, CPU, uTP, and whether the MIME table resolves `.mkv`/`.mp4`
- **config:** the effective config's fingerprint
- **scenario:** params and seed
- **metrics:** `{name: {kind: exact|count|timing, unit, better, values[], median, p95}}`
- **checks:** `[{id, status: pass|fail|xfail|xpass|n/a, known, evidence}]`
- **per-session:** requests (range, status, ttfb, bytes, aborted, handler_lag) and player stats
- **torrent:** waste and residual download
- **resources:** peak goroutines, leaked stacks by label, peak readers, peak heap
- **findings**
- **artifacts:** their paths

**Metric kinds.**

| Kind | Metrics |
|---|---|
| Exact | integrity errors, panics, race reports, leaked goroutines, quiescence counts (CDN bodies and streamer slots included), readers created, torrents loaded by PROPFIND, HDRezka link resolves and invalidations, ETag changes |
| Count | reads, debounced and seek updates per MiB, actual vs redundant seeks, downgraded pieces, waste ratio, residual download, read timeouts, slow reads, HDRezka upstream opens (under faults a dropped request may open its body or not), hash failures |
| Timing | TTFB, time to playable, seek latency p50/p95, stall count and duration, rebuffer ratio, activation wait, `handler_lag_after_abort`, PROPFIND latency, mutex wait |

**Compare.** `compare base head` takes two runs (a run's folder, its `index.json`, or one `report.json`) and writes `streamlab.compare/v1` (`compare.json`) and `compare.md`, whose first line is the verdict. The verdict is one of `pass | regression | improvement | inconclusive | fail`; the worst scenario's wins.
- **fail:** head has an integrity error, panic, race, watchdog trip or leak (its `integrity`, `http`, `race`, `watchdog` or `quiescence` check fails, whatever base did), or a repetition that hung or crashed (a warm-up's too); or a check that wasn't fail in base is fail in head, on every head rep or on significantly more of them than base's (Fisher's exact test over the reps whose failure carries no known signature). A check that fails on both sides is judged the same way, so a base flake (one rep, or #195 over its rate) can't hide a head that fails every rep: head failing on every rep where base didn't, or on significantly more, fails; about as often passes. On fewer it's `inconclusive`: the lab has rare failures no signature explains yet (a body broken right after its first chunk; a window lowered outside F13's pieces), on base and head alike, and one such rep can't tell a regression from them. Checks are judged as the runner left them, so a known issue's xfail (D6) never fails a compare, and a check the runner failed with no known issue counts every failing rep, whatever signature it carries (`fail:F11` outside F11's guards). A correctness check that couldn't tell in head (pass → n/a), or that ran in base only, is `inconclusive`: it can't vouch for head. Every check base ran is listed.
- **Exact metrics:** any change on any rep is reported (the sides' sorted values differ), its direction from `better` and the means; values that moved both ways with equal means are only reported. A worse value is a regression, unless the check that judges it fails (already a fail) or is an xfail of an intermittent issue (#225 moves `leaked_goroutines` on the reps it fires). A deterministic issue's value is stable, so its change counts.
- **The lab's own metrics** (`observe_overhead`, S5's cost) are reported, never a verdict.
- **What head didn't reach** is `inconclusive`: a metric with a better direction that base measured and head didn't, or one only the reps that got there record (TTFB, time to playable, the next episode's, resume TTFB, seek latency) measured on fewer of head's reps than base's. A head that never gets playable has no time to playable, and its best reps against all of base's aren't a comparison. The confirmation keeps both rules, over each run and both together.
- **Δ** for counts and timings is the Hodges–Lehmann shift, the median of every head − base difference (the Mann-Whitney test's own estimate), against base's median for the percentage. Lab timings often fall in two clusters (a cold start connects at once or a second later), and when both sides mix them alike, the difference of the medians swings by the gap while the shift doesn't.
- **Count metrics:** flagged when |Δ| > max(10%, 3×CV, a floor of one event, 1 MiB, 0.05 for ratios, 64 KiB/s). The CV is the machine's from `calibrate`, or 0.10 when there's none, and `compare.md` says which. A flagged change also needs 5 reps a side and Mann-Whitney p < 0.05, as a timing does, or it's `inconclusive`: the lab's counts move with timing more than D5 hoped (a waste ratio over a few bytes served, updates per MiB), and one rep of `etag-stability` an unchanged tree once read as +75% waste.
- **Timing metrics:** need ≥5 reps. A Δ over max(15%, an absolute floor per metric: 100 ms TTFB, 200 ms time to playable, 1 stall, 500 ms stall time, 0.02 rebuffer ratio …, `internal/streamlab/compare/rules.go`), with Mann-Whitney U p < 0.05 (the exact permutation distribution up to 60 values on both sides together, ties included; the normal approximation past it), gives regression or improvement. Anything weaker is `inconclusive, add reps`. Race builds (`env.race`) get no timing verdict (D8).
- **Intermittent known issues** (#195, #225). A head check that fails only because such an issue fired over its rate is judged against base: a **fail** when it fired on every head rep (D6: no rate explains that; unless it fired on every base rep too, outside a correctness check: a loaded machine) or significantly more often in head than in base (Fisher's exact test, p < 0.05, per scenario and over the run), else **inconclusive**. Base ran under the same load, so a loaded machine that makes #195 fire on both sides can't flip a verdict. An issue within its rate that fires significantly more often in head is a regression. Skewed load (below) makes these `inconclusive`, except on every head rep or on a correctness check (#225's leak on `quiescence`, #195 on `watchdog`). A base that failed only because such an issue fired over its rate isn't fixed by a head where it didn't: the firings are judged instead.
- **xfail → xpass** is an improvement: "known issue X appears fixed (remove it from the registry)".
- **Machines.** Two machines (host, OS, architecture, CPU model and count) are refused without `--force`; forced, only checks and exact metrics are judged. Different params are refused the same way.
- **Load.** Each repetition's 1-minute load average per CPU is in its `env.load`. When the sides' medians differ by more than 0.5 per CPU and 1.5×, timing and count changes are `inconclusive` (skewed load). A median over 1.0 per CPU is noted as high load.
- **Exit codes:** 0 pass or improvement, 1 fail or regression (also when a side couldn't run another scenario), 2 inconclusive, 3 tool or environment error (a side that couldn't run a scenario, when nothing failed).

One compare runs a few hundred tests (every timing and count metric of every scenario), so a handful reach p < 0.05 by chance. A single compare is therefore a screen: `compare base head`, `ab --confirm=false` and an ab folder without its confirmation turn their timing and count regressions and improvements, and a rise of an intermittent issue within its rate, into `inconclusive` ("a single compare is a screen: confirm with ab", exit 2). `ab` **confirms** every scenario whose verdict rests on statistics: it runs the scenario again on fresh repetitions (as many a side, after their warm-ups), and a timing or count change counts only when each run shows it on its own, one-sided the way it went, and both runs' repetitions together back it. The screen must reach p ≤ 2/252 and the fresh repetitions p ≤ 1/252: at 5 reps a side, at most one pair out of order, and none. On an unchanged tree a chance flag replicates with probability P(screen) × P(fresh), and the null simulation (`compare/null_test.go`: 22 scenarios × 15 independent, noisy metrics, 5 reps a side) gave a regression in 3.5% of unchanged abs with the fresh repetitions at 2/252 and the screen at 0.05, 2.1% at 1/252, and 1.2% with both (10,000 abs; `STREAMLAB=1` holds 3,000 to 2%). Holm over the flagged metrics can't do it at 5 reps: at a family-wise 2% with six flags it asks for p ≤ 0.0033, below the 1/252 five reps reach, so a regression that moves six metrics (356e74d) would confirm none. A change the screen showed less strongly stays `inconclusive`: add reps. A screen that wasn't significant makes the fresh repetitions a second screen, not a replication, so it stays `inconclusive` whatever they show (an unchanged tree once reached `regression` that way: time to playable in `parallel-ranges`, whose reps cluster at 2.5, 5, 7.5 and 12–17 s). A change both runs together put within the threshold is noise (pass); any other disagreement is `inconclusive`. Intermittent issues and flaky checks are judged on both runs' reps together (a rise within an issue's rate holds only when the fresh reps show it on their own too; a run-wide one, which the confirmation can't rerun, stays `inconclusive`), and a fail in the confirmation counts.

**`ab --base main [--head rev] [--reps 5] [--scenario … | --tier gate] [--race]`:**
1. Checks the base out in a worktree, `.streamlab/worktrees/<sha>` in the main checkout (`git rev-parse --git-common-dir`'s parent when that's the checkout's own `.git`, which every worktree of the repo shares; a bare repository's worktree or a submodule keeps its own), claimed by a lock on `<sha>.lock` for the run's life, `--keep` included (`<sha>-<pid>` when another ab holds it). A `.streamlab` the tool creates gets a `.gitignore` of `*`, so a checkout on an older branch doesn't list it. The head is the working tree, uncommitted changes included, or `--head` in a worktree of its own. `--out` must be new or empty.
2. Refuses a base without H1–H3 ("base predates streamlab hooks"), copies the working tree's `internal/streamlab` and `cmd/streamlab` (uncommitted changes included) over it, and over `--head`'s worktree too (so both sides use the same lab), and builds both side by side (`-tags disable_libutp`; `-race` too with `--race`, which also adds the race check and runs the stress tier by default).
3. Interleaves the runs, each repetition a fresh subprocess whose working directory is its side's tree, so each records its own commit: the warm-ups, then rep 1 of each side, rep 2 … with each pair led by the other side than the last (AB BA AB …), so a load that rises or falls through the run weighs on both alike. An intermittent issue that needs confirming on either side (D6) gets its confirmation reps on both.
4. Samples the load average before every repetition, writes `ab.json` (the builds, commits and schedule) before comparing, so a run whose comparison fails can be judged again (`compare <ab folder>`), compares, confirms (above), writes `compare.json` and `compare.md`, and removes its worktrees and binaries (no `git worktree prune`: other worktrees' records stay).

**`calibrate [--rev main] [--reps 10]`** runs the revision (with the working tree's lab, in a worktree like ab's base) and stores each scenario's metrics' CVs in the main checkout's `.streamlab/noise.json` under this machine, with the commit, date, reps and load. Scenarios that didn't pass are skipped: their numbers measure the failure (exit 1; 3 when the tool couldn't run one). `compare` and `ab` read it for this machine; `compare.md` says when there's none.

**Baseline.** `internal/streamlab/testdata/baseline.json` (`streamlab.baseline/v1`) holds, per gate scenario, each check's expected status (`pass`, `n/a`, `xfail:<issue>`) and each exact metric that every rep measured with one value; never a timing. An intermittent issue that happened to fire while it was recorded isn't an expectation.
- `streamlab gate` runs the gate tier and holds it to the baseline (`gate.json`, `gate.md`): it fails when the run fails, an exact metric moves or isn't measured, a check leaves its expected status or doesn't run (a scenario's checks are fixed: one gone means the lab changed under the baseline), or an xfail turns xpass (the issue appears fixed: the registry and the baseline must say so). An intermittent issue within its rate, a check that couldn't tell (n/a, whatever was expected: F3, F6, F11, F12, F13 and #232 have timing-dependent n/a paths), and an expectation that depends on the host's MIME table (F1) recorded on another host are only noted.
- `baseline update --reason "…" [--from run]` is the only way to change it. It refuses a failing run and one where a known issue appears fixed (xpass: the registry changes first), records the reason, commit and date in the file's history, prints what changed, and writes a digest of the history and the expectations: the gate refuses a file edited by hand, and an update of an edited file must re-record every scenario in it, so an edit isn't put under a fresh digest by updating another scenario.
- `affected [--base main]` maps the changed files (`git diff --no-relative` from the merge base, uncommitted and new files included) to scenarios: by guard (a commit a scenario guards changed the file, or a known issue it guards lives there: the focus), by a table from the streaming path's packages and the library to the scenarios that run them, and any other package the lab's binary links (`go list -deps ./cmd/streamlab`) to the whole gate tier; the lab itself, go.mod and the config map to every scenario. It prints the ab and check commands, and warns when one diff changes code the lab runs and the lab itself (anything under `internal/streamlab` or `cmd/streamlab`: what it measures as much as its thresholds, signatures, scenarios and baseline) together: ab runs one lab on both sides, so a looser lab would hide a regression.

**D7 and the unchanged tree, as measured (#179).** `ab --base main --reps 5` on scratch branches, base `main` with HEAD's lab:
- **0d364a2** (`--race`, the stress tier's read scenarios): **fail**. `seek-pending-read` delivered about 0.5 MB of wrong bytes (integrity) and raced on anacrolix's `reader.readContext`, on both reps.
- **356e74d**, as reverted (`UpdatePriorities` after the activation wait): **inconclusive**. `InitialPrioritize` already wants the header and footer before the wait, so peers connect at once (`activation_wait` ~33 ms on both sides), and anacrolix's small first request window makes the late urgent window cost little. Its old behaviour by hand (the wait before the reader exists, so nothing is wanted during it): **regression**, confirmed. cold-start `activation_wait` 33 → 3334 ms and `probe_latency` 517 → 5231 ms; seek-back `ttfb` 1324 → 5984 ms.
- **9de9c67**: **inconclusive**. Stalls don't change, in slow-swarm too; only `pieces_downgraded` rises by a third (p 0.008), and it has no better direction. #265 adds a signal.
- **Unchanged tree:** three runs of the whole gate tier side by side, each 22 scenarios × 6 reps a side plus confirmations, give **inconclusive** three times (13–17 scenarios pass, the rest inconclusive), never a regression or a fail. Loads were 0.35–0.37 per CPU on both sides. An earlier set of three gave one `regression`, which led to two rules above: a confirmation replicates only a significant screen, and a check that fails on some reps is judged by them.
- **Rejudged with #267's review rules** (`compare <ab folder>`: a single compare is a screen, each run one-sided at 2/252 and 1/252, what head didn't reach is inconclusive, …): all six unchanged-tree runs are `inconclusive` (13–18 scenarios pass), the by-hand 356e74d stays a confirmed `regression` (its six flagged metrics fully separated in both runs), 0d364a2 a `fail`, and 356e74d as reverted and 9de9c67 `inconclusive`.

S6 (#177) changed how lab seeders limit their rate: from anacrolix's `UploadRateLimiter` to paced storage reads. Timing medians from before S6 aren't comparable with later ones, so S8's (#179) baselines and calibrations must be taken after S6.

## 7. streamtrace: the flight recorder

`internal/streamtrace` is for production, where the lab can't reach. Its rules:

| Rule | |
|---|---|
| R1 | When off, a hook is one atomic load. Recording is lock-free and lossy: a fixed ring of 64Ki events (~4 MiB), no allocation, no `fmt`, strings interned at open. |
| R2 | A hook never takes `fs.mu`, `TorrentFile.mu`, `PriorityReader.mu`, `Prioritizer.mu` or the anacrolix client lock, and never calls anacrolix. |
| R3 | A hook is one line after an existing statement, reusing values already computed. No reordering, no new error path, no new goroutine in the read path. |
| R4 | Standard library only. `STREAM_TRACE=off\|summary\|detailed`. `summary` stays under ~50 events/s during playback; `detailed` adds per-read events for the lab. |
| R5 | Every hook PR passes a lab A/B at off, summary and detailed. Exact metrics are identical, timings are within noise, `BenchmarkEmit` makes 0 allocs; off costs ≈ 1 ns; a recorded event costs about one monotonic clock read, plus one atomic add, plus ≤10 ns of ring work (≈30–35 ns on arm64; amd64 unmeasured); and event counts reconcile with the Prometheus counters. |

**Hook sites:**
- **HTTP:** an outermost `traceMiddleware` (request ID, Range, If-Range, UA, status, bytes, TTFB, abort), and `webdavFile.Seek`.
- **VFS:** `LibraryFS.OpenContext`.
- **TorrentFile:** `ReadAt`, the `readContext` branches, the drain, `waitForFirstAccess`, `Close`.
- **PriorityReader:** create, close, `detectFormat`.
- **Prioritizer:** `InitialPrioritize`, `UpdateForSeek`, `SetFormatInfo`, `ResetPriorities`, with ranges, downgraded counts and loop duration.
- **Torrent service:** add and remove.
- **ActivityManager:** state changes and activation.
- **Sessions:** begin and end.
- **RemoteFile:** open, reuse, skip, reopen, timeout.
- **HDRezka streamer:** open, refresh, retry, slot wait.

**Endpoints.** They live on the existing loopback metrics server, through a `metrics.NewServer(..., WithDebug(...))` option:
- `/debug/pprof/*`
- `/debug/stream/events?since=` (NDJSON)
- `/debug/stream/state`, built from `ReaderTracker` via TryLock, `svc.Detail`, `am.GetState`, sessions and the gauges, at most 1 request/s
- `/debug/stream/pieces?hash=`
- `/debug/stream/client`

`LOG_LEVEL` (default INFO) makes the existing DEBUG streaming logs reachable.

**Open decision, settled in #187 with overhead data.** Should production run `summary` by default whenever `METRICS_ENABLED` is set? The proposal is yes: a recorder must already be on when a stall happens.

## 8. Production, read-only

The user opens the tunnel (`ssh -N -L 9090:localhost:9090 momoshtrem-v2`) and sets `METRICS_ENABLED`/`STREAM_TRACE` in the VM's `.env`.

| Command | What it does |
|---|---|
| `capture --target http://127.0.0.1:9090 --for 10m` | Collects events by sequence number, state every 5 s, metric deltas, goroutine profiles at start and end, and optionally a CPU profile. Refuses non-loopback targets. |
| `timeline <capture>` | Per-session lanes: requests, seeks, inferred stalls, and piece arrival against the read position. |
| `watch --target …` | A live view while the user plays on the Apple TV. |
| `replay <capture or recording>` | Maps the requests onto lab media of the same size and piece length. |
| `record --listen :36912 --upstream <webdav>` | A recording proxy on the Mac. The user points Infuse at it briefly, so the client model rests on what Infuse really sends. |
| `run --target <webdav>` | Black-box mode for **local compose only**: against production it would load and download torrents. |

## 9. Slices

**Phase 1: the safety net.** No production behaviour changes.

| Issue | Slice | Depends on |
|---|---|---|
| #172 | S1 media + swarm, and the first piece-transfer test (no production files) | |
| #173 | S2 hooks H1–H3 | |
| #174 | S3 stack, client, and the integrity, HTTP and quiescence checks | #172, #173 |
| #175 | S4 player, gate catalog, runner, report, CLI, known-issue registry | #174 |
| #176 | S5 observe: samplers, piece timeline, profiles, watchdog, findings | #175 |
| #177 | S6 swarm faults and the fake HDRezka CDN | #175 |
| #178 | S7 `check`: race build and the stress tier | #175 |
| #179 | S8 `compare`, `ab`, `calibrate`, baseline, `gate`, `affected`; mutation validation | #175, #178 |
| #180 | S9 skill and README | #179 |
| #181 | S10 CI job (non-blocking, then required) | #179 |

**Phase 2: fidelity.**

| Issue | Slice | Depends on |
|---|---|---|
| #182 | S11 Infuse recorder and replay format | #175 |
| #183 | S12 H4 and tree-race scenarios (F4, F5) | #174, #178 |
| #184 | S13 realism tier and black-box mode | #175 |

**Phase 3: streamtrace.**

| Issue | Slice | Depends on |
|---|---|---|
| #185 | S14 package and ring, no call sites | |
| #186 | S15 debug endpoints and `LOG_LEVEL` | #185, #179 |
| #187 | S16 hooks part 1 (HTTP, VFS open, TorrentFile, PriorityReader, Prioritizer) | #185, #179 |
| #188 | S17 hooks part 2 (activity, service, sessions, RemoteFile, HDRezka), shared wiring | #187 |

**Phase 4: production, read-only.**

| Issue | Slice | Depends on |
|---|---|---|
| #189 | S18 `capture`, `timeline`, `watch` | #186, #187 |
| #190 | S19 replay captures, prod playbooks | #189, #182 |

**Overlapping issues:**
- #83: F3, and the HDRezka metrics (S17 coordinates).
- #73: F2.
- #28: VFS tree robustness (S12).
- #56: test deadlocks on the shared Postgres. The lab needs no Postgres.
- #77: smart caching. The lab measures waste and residual download.
  - Its only change on the streaming path is H5 (#201): the pinnable piece cache.
  - Background downloads set wanted files to Normal priority (`File.SetPriority`), outranked by the stream's Now and Readahead windows. While anything streams, they pause for other torrents (`pause_while_streaming`).
  - #208 adds the `background-download` and pinned `small-cache` scenarios, which check that default with numbers. Contract: redesign PLAN §6 "Storage (#77)" and "Downloads (#77)".

## 10. Verification

- **Every slice.** `cd momoshtrem && go vet ./... && go test ./...` stays green, and the lab packages pass `go test -race -tags disable_libutp`.
- **Phase 1 is done when:**
  - `go run ./cmd/streamlab gate` passes on main.
  - `streamlab check` reports races as JSON.
  - `streamlab ab --base main --reps 5` on an unchanged tree never gives a regression, three runs in a row.
  - Each D7 revert is flagged. (#179: 0d364a2 and 356e74d's old behaviour are; 9de9c67 waits on #265.)
  - The `scrub` report shows F12 as an xfail.
- **Phase 3 is done when:** an A/B at `STREAM_TRACE` off, summary and detailed gives identical exact metrics, and the event counts reconcile with Prometheus.
- **Phase 4 is done when:** a 10-minute production capture during an Infuse session renders a timeline and replays in the lab.

## 11. Risks

| Risk | Mitigation |
|---|---|
| Lab fidelity: loopback has no RTT, and one seeder differs from a real swarm. | The proxy, multiple seeders, the realism tier, the Infuse recorder and capture/replay. The lab is a regression detector, not an absolute predictor. |
| Flaky results train agents to ignore them. | Deterministic correctness checks, statistics-only timing verdicts, calibrated noise floors, and `inconclusive` instead of a false regression. |
| The lab's wiring drifts from `main.go`. | S17's optional shared `internal/streamwire`. |
| Phase 1's slog capture depends on log text. | streamtrace replaces it. |
| Samplers take the anacrolix client lock (`PieceStateRuns`). | Low frequency, and each sampler reports its own overhead. |
| Agents game the gate by editing baselines. | `baseline update --reason`, an `affected` warning, and the skill's rule. |
