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
| D6 | **Known issues are expected failures.** A check that fails because of a known bug (§3) reports `xfail` and doesn't block. When a fix lands, it flips to `xpass` and the report says so. A known issue explains a failure only when the failure carries its signature (a #195 stall is a timed-out read at a wanted, incomplete piece with the seeder connected; an F12 lag ends within the read timeout), and only at the rate it's known for. An intermittent issue (#195) that fires on every repetition, or on more than its rate (25% by default), is a **fail**: with one repetition, one firing is every repetition. A deterministic suspect (F2, F12, …) fires on every repetition by nature, so its entry says so and relies on its signature to stay narrow. The registry is `internal/streamlab/known`. |
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

## 4. Production-code hooks

All are additive or pure refactors.

| Hook | Change | Slice |
|---|---|---|
| H1 | `vfs.NewStaticFS([]StaticMovie, []StaticEpisode) *LibraryFS` in `internal/vfs/static_fs.go`, moved out of `export_test.go`. `export_test.go` keeps `NewTestFS`/`TestMovie`/`TestEpisode` as aliases. | #173 |
| H2 | `torrent.ClientConfigFor(cfg, cc) *atorrent.ClientConfig`. `NewClient` delegates to it, and a parity test covers it. | #173 |
| H3 | `ActivityManager.SetCheckInterval(d)`, set before `Start`. It is a fixed 30 s today. | #173 |
| H4 | A `build func() *DirectoryTree` field on `LibraryFS`: `buildTreeFromDB` by default, a static builder from `NewStaticFS`. `rebuildTree` calls `fs.build()`. This lets the lab invalidate the tree, which F4 and F5 need. | #183 |

The leecher finds the seeder through the real `service.add()` path: the magnet carries `x.pe=<seeder addr>`, which anacrolix maps to `PeerAddrs` (`spec.go:42`).

Cold start is faithful. After `add()` sets every piece to None, anacrolix dials no peers until the first read raises priorities. The activation wait therefore measures real dial and handshake time when the info dict comes from the `MetainfoStore`. With metadata from peers, the connection that fetched it stays open.

## 5. The lab

```
momoshtrem/cmd/streamlab/              CLI: run, list, explain, gate, check, ab, compare, calibrate,
                                       affected, record, replay, capture, timeline, watch
momoshtrem/internal/streamlab/
  media/     f(seed, offset) content (splitmix64), MKV / MP4 (moov-at-end) overlays, multi-file layouts,
             metainfo builder + cache (os.UserCacheDir()/momoshtrem-streamlab)
  swarm/     generated-storage seeders (UploadRateLimiter), TCP proxy (latency, jitter, bandwidth, drop,
             vanish), discovery delay, corrupt piece, metadata-from-peers
  cdn/       fake HDRezka CDN (httptest + ServeContent over media; 403 expiry, 503 bursts, slow/drop),
             fake hdrezka.LinkResolver, wired through the real Streamer and NewHDRezkaSource
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

**Leecher storage.** The leecher uses the real `torrent.InitStorage`, the filecache LRU that production uses.

**Stack.** It is wired exactly as `cmd/momoshtrem/main.go` does:
1. `InitStorage`, then the leecher.
2. `NewActivityManager` → `NewMetainfoStore` → `NewService(WithMetainfoStore, WithPieceStorage, WithSpeedSampler)`.
3. `NewStaticFS` → `SetTorrentService`, `SetReaderTracker`, `SetSessions(NewRegistry(WithClock))`, `SetMetrics(metrics.New(perRunRegistry))`, `SetRemoteSource` (it comes with S6, #177).
4. `webdav.NewServer(...)`, `SetMetrics` (the same metrics; without it, `requests_inflight` stays 0 and the quiescence check passes trivially), then `.Handler()`.

The config is `config.DefaultConfig()` with an optional `--config prod.yaml` overlaid. A fingerprint of the effective config goes into every report.

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
| `slow-swarm` (stalls expected), `seeder-loss`, `corrupt-piece` | |
| `small-cache` | F11 |
| `etag-stability` | F2 |
| `hdrezka-{play,seek,expiry,503,stall}` | #66 |

**Stress tier** (`check`, `-race`, seeded):
- close during a blocked read
- seek during a pending read
- remove, pause or idle during a read
- concurrent adds of one hash
- many readers of one file
- a client abort during a blocked read (F12)
- `InvalidateTree` during opens (after H4: F4, F5)

**Realism tier:** Sintel or Big Buck Bunny in-process with real DHT and trackers. Its reports use the same schema and never feed the gate.

### Checks

| Check | Passes when |
|---|---|
| Integrity | Zero mismatched bytes. |
| HTTP | A Range GET answers 206 with the right `Content-Range` and `Content-Length`, and no handler panics. |
| Quiescence | After the last request, within the settle time: `ReaderTracker.Count()==0`; the `open_files`, `blocked_reads` and `requests_inflight` gauges are 0; no `slreq`-labelled goroutine running momoshtrem code, or started by momoshtrem on a library function (anacrolix-rooted ones a request started, such as peer connections and hashers, are reported as inherited); `sessions.Count()==0` after skewing the session clock 31 s; zero open CDN bodies. This is the leak signal that matters, because production never tears down. |
| Priority invariants | Stated over the **effective** priority of incomplete pieces while downloads are allowed: `Piece.State()` can't read the explicit priority. During playback, `[pos, pos+urgent)` is Now. `residual_download_bytes` measures what downloads after the last close. |

**Watchdog.** If requests are active, no fault is running, and the client has made no progress for N s, it dumps:
- the labelled goroutine profile
- a `runtime/trace` `FlightRecorder` snapshot
- `client.WriteStatus`
- the piece runs

**Findings.** Rules that turn data into hints, each pointing at evidence. For example, "stall while the playhead's piece was below Now" points to the prioritizer, and "handler lived N s after the client aborted" is F12.

## 6. Reports and verdicts

**Layout.** A run writes `.streamlab/runs/<id>/` at the repo root (or `--out`): `index.json` (`streamlab.index/v1`) and `index.md`, one row per scenario; per scenario, `report.json` and `report.md`, and a folder per repetition (`warmup-1/`, `rep-1/`, …) with the child's `rep.json`, its `child.log`, `goroutines.txt` after a hard timeout, and both clients' status at every timed-out read (`failure-N.txt`). `run` exits 0 when every scenario passes (xfail included), 1 when one fails or a repetition timed out or crashed, 3 on a tool or environment error.

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
| Exact | integrity errors, panics, race reports, leaked goroutines, quiescence counts, readers created, torrents loaded by PROPFIND, HDRezka upstream opens, ETag changes |
| Count | reads, debounced and seek updates per MiB, actual vs redundant seeks, downgraded pieces, waste ratio, residual download, read timeouts, slow reads |
| Timing | TTFB, time to playable, seek latency p50/p95, stall count and duration, rebuffer ratio, activation wait, `handler_lag_after_abort`, PROPFIND latency, mutex wait |

**Compare.** The verdict is one of `pass | regression | improvement | inconclusive | fail`.
- **fail:** head has an integrity error, panic, race, watchdog trip or leak; or a check that was pass or xfail in base is fail in head.
- **Exact metrics:** any change is reported.
- **Count metrics:** flagged when |Δ| > max(10%, 3×CV from `calibrate`).
- **Timing metrics:** need ≥5 reps. A median Δ over max(15%, an absolute floor), with Mann-Whitney p < 0.05, gives regression or improvement. Anything weaker is `inconclusive`.
- **Exit codes:** 0 pass or improvement, 1 fail or regression, 2 inconclusive, 3 tool or environment error.

**`ab --base main`:**
1. Makes a worktree under `.streamlab/worktrees/`.
2. Copies HEAD's lab code over the base (so both sides use the same lab) and builds both.
3. Interleaves the runs ABAB, each in a fresh subprocess.

Comparing reports from different machines is refused without `--force`.

**Baseline.** `testdata/baseline.json` holds only exact and correctness expectations. `baseline update --reason` is the only way to change it. `affected` warns when one diff touches both streaming code and lab thresholds.

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

## 10. Verification

- **Every slice.** `cd momoshtrem && go vet ./... && go test ./...` stays green, and the lab packages pass `go test -race -tags disable_libutp`.
- **Phase 1 is done when:**
  - `go run ./cmd/streamlab gate` passes on main.
  - `streamlab check` reports races as JSON.
  - `streamlab ab --base main --reps 5` on an unchanged tree never gives a regression, three runs in a row.
  - Each D7 revert is flagged.
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
