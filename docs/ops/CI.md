# CI and images

> Runbook · 2026-10-15 · #36 · Epic #32 · Plan: [`PLAN.md`](PLAN.md) D7, §4 · streamlab (§7): #181
> `.github/workflows/ci.yml` checks every PR and publishes images for every `main` commit. The VM never builds.

## 1. What runs when

| Job | On | Does |
|---|---|---|
| `go` | PRs and `main` | `go vet ./...` and `go test ./...` in `momoshtrem/`. A throwaway Postgres 16 service is set as `MOMOSHTREM_TEST_DATABASE_URL`, so the integration tests run instead of skipping |
| `web` | PRs and `main` | `npm ci`, `npm run lint`, `npx tsc --noEmit` and `npm test` in `web/`. Node is the major in `web/Dockerfile`, so CI tests what ships |
| `image (momoshtrem)`, `image (momoshtrem-web)` | PRs | Build each image for `linux/amd64` without pushing. They run alongside the checks |
| `publish (momoshtrem)`, `publish (momoshtrem-web)` | `main` | Wait for `go` and `web`, then build and push the image (§2) |
| `streaming-paths` | PRs and `main` | Says whether a PR changes the streaming path, for `streamlab` (§7). On `main` the answer is always yes |
| `streamlab` | PRs that change the streaming path, and `main` | Does three things (§7):<ul><li>`streamlab affected --strict` (PRs)</li><li>`go test -race` over the lab and the streaming packages</li><li>`streamlab gate --reps 1` against the committed baseline</li></ul>**Non-blocking** for now |

The commands are the ones in `CLAUDE.md` "Commands". A new push to a PR cancels that PR's older run. `main` runs are never cancelled, so every commit gets its images.

**A green `main` commit** is one whose `publish` jobs succeeded. Its images exist, and its checks passed on that exact commit. `ops/deploy/deploy.sh` deploys only these: it checks GHCR for both images first ([`DEPLOY.md`](DEPLOY.md)). GitHub never deploys (PLAN D3).

## 2. Images

| Image | Built from | Notes |
|---|---|---|
| `ghcr.io/shapedtime/momoshtrem:<sha>` | `momoshtrem/Dockerfile` | `VERSION=<sha>`, so `/api/status` reports the full commit. Settings shows the first 7 characters |
| `ghcr.io/shapedtime/momoshtrem-web:<sha>` | `web/Dockerfile` | |

- **Tags are the full commit SHA, and only that.** There's no `latest`.
- **Tags are immutable.** `publish` skips any tag that already exists, so re-running a workflow never replaces an image that may already be deployed.
- **Images are public** (D7), so pulling needs no login:
  ```bash
  docker pull ghcr.io/shapedtime/momoshtrem:<sha>
  ```
- **Layer cache.** `main` writes it to the GitHub Actions cache, one scope per image. PR builds only read it.
- **Old images are kept.** Public images cost nothing to store (PLAN §11).

## 3. Public-repo rules

- **Runners.** Jobs run only on GitHub-hosted runners. There's no self-hosted runner, so a fork's PR can't run code on our machines (D3).
- **PR runs.** PRs use the `pull_request` event, never `pull_request_target`. A fork's PR gets a read-only token and no secrets.
- **The token.** It defaults to `contents: read`. Only `publish` gets `packages: write`, and it pushes with the built-in `GITHUB_TOKEN`. GitHub holds no other secrets (PLAN §7).
- **Actions.** Workflows use only `actions/*` and `docker/*`, pinned to a commit SHA with the version in a comment. Checkout doesn't keep the token (`persist-credentials: false`).

## 4. Dependabot

`.github/dependabot.yml` opens one grouped PR a week for each ecosystem:
- **GitHub Actions.** It moves the SHA pins and their version comments.
- **The Dockerfile base images** in `momoshtrem/` and `web/`. A Node bump in `web/Dockerfile` also moves the `web` job's Node.
- **The third-party images in `compose.yml`**: Jackett, FlareSolverr, Postgres, pgAdmin and Caddy, pinned to exact tags (#37, #106). Our own two images are ignored: deploys move their tag.

## 5. One-time setup

The user does these steps in GitHub. Agents can't.

1. **Make the packages public**, after the first `publish` run:
   - GHCR creates new packages as private, and there's no API to change it.
   - For each of `momoshtrem` and `momoshtrem-web`, go to your profile → **Packages** → the package → **Package settings** → **Danger Zone** → **Change visibility** → **Public**.
   - This can't be undone, which is fine: the code is public too.
2. **Require the checks on `main`**:
   - Go to **Settings** → **Rules** → a branch ruleset on the default branch → **Require status checks to pass**.
   - Add `go`, `web`, `image (momoshtrem)` and `image (momoshtrem-web)`.
   - `streamlab` joins them once it has been green on `main` for a week (§7).
3. **Approve outside contributors' PR runs**: **Settings** → **Actions** → **General** → **Require approval for all external contributors**.

## 6. Changing CI

- **Keep PR checks fast.** v2 agents open PRs often.
  - `streamlab` takes ~22 minutes, so it runs only on PRs that change the streaming path (§7).
  - Playwright E2E can come later as its own job: `npx playwright install --with-deps chromium`, then `npx playwright test`.
  - Add it to the required checks only once it's stable.
- **Renaming a job renames its check.** Update the required checks (§5.2) in the same change, or PRs will wait forever on a check that no longer runs.
- **Pin new actions by SHA**: `gh api repos/<owner>/<repo>/commits/<tag> --jq .sha`.

## 7. streamlab

`streamlab` is the streaming path's safety net (`docs/streamlab/PLAN.md` S10, #181). The streaming path is the code that took months of tuning on the Apple TV.

### When it runs
`streaming-paths` diffs a PR's merge commit against its first parent, the base it merges into. `streamlab` runs when one of these changed, all under `momoshtrem/`:
- `internal/{vfs,webdav,streaming,torrent,sessions,hdrezka,streamlab,streamtrace}/`
- `internal/service/hdrezka_source.go`
- `cmd/momoshtrem/main.go`
- `cmd/streamlab/`
- `go.mod`, `go.sum`
- `internal/{config,common,metrics}/`: `streamlab affected` maps them to every scenario. The config holds the streaming defaults (readahead, `read_timeout`).

It also runs when `.github/workflows/ci.yml` changes.

`internal/library/` is left out: v2 changes it all the time. On other PRs the check shows as skipped. On `main` it runs after every push, so its record there shows how often it fails.

### What it does
It works in `momoshtrem/`, on a checkout with full history:
1. **Build** `cmd/streamlab` with `-tags disable_libutp`.
2. **`streamlab affected --strict --base origin/<the PR's base>`** (PRs only). It warns when one diff changes the code the lab runs and the lab itself, which includes the baseline and the known-issue registry (PLAN §6 "affected", §11).
   - The warning goes to the job summary and as an annotation.
   - It never fails the job: a fix that retires a known issue rightly changes the code, the registry and the baseline in one PR.
   - What it asks for: a reviewer checks that the lab change doesn't loosen what would catch the streaming change, and the PR says why the two land together.
   - It runs before anything else touches the working tree, because affected diffs the working tree. The base is the PR's real base, fetched with the history. A stale local `main` doesn't come into it.
3. **`go test -race -tags disable_libutp -count=1`** over `./internal/streamlab/... ./cmd/streamlab/... ./internal/vfs/... ./internal/streaming/... ./internal/torrent/... ./internal/sessions/...`. `-count=1` because a cached pass catches no race.
4. **`streamlab gate --reps 1`**, even when the tests failed.
   - It runs the gate tier, one warm-up and one rep per scenario, and holds it to `internal/streamlab/testdata/baseline.json`.
   - It judges counts and correctness, never timings (PLAN D5).
5. **Reports.** `gate.md` goes to the job summary, and every report is uploaded.

It takes **~22 minutes** on the 4-vCPU runner (#269): the tests 2, the gate 18 for 22 scenarios. `timeout-minutes: 40` leaves room for two things:
- **Confirmation reps.** When an intermittent issue fires, the scenario runs 2 more.
- **Hard timeouts.** 3–4 minutes a rep.

### The gate on a CI runner
The baseline was recorded on macOS. The gate tells hosts apart only by their MIME table (F1): when `.mkv` resolves differently from the baseline's host, it only notes F1's expectations.
- **ubuntu-24.04** resolves `.mkv` and `.mp4`, as macOS does. So every expectation holds there, F1's included (#269: PASS, 22 of 22, no notes).
- **The other differences** are the host, the CPU and Linux. The baseline holds no timings, so they don't matter.
- **Production's `alpine:3`** has no MIME table. The gate fails there on F1's knock-on effects that the lab doesn't know yet (#271). A job for it waits on that (#272).

### Reading a result
- **The job summary.** On PRs, affected's warnings come first. Then `gate.md`: the verdict on its first line, and a row per scenario with its problems and notes.
- **The artifact `streamlab`**, kept 14 days: `gh run download <run-id> -n streamlab -D /tmp/streamlab-<run-id>`.

| File | What |
|---|---|
| `affected.txt`, `affected-warnings.txt` | The scenarios the diff touches, the commands to run, and the warning (PRs) |
| `go-test-race.txt` | go test's output. A race is a `WARNING: DATA RACE` block |
| `gate/gate.md`, `gate/gate.json` | The verdict against the baseline, per scenario |
| `gate/index.md`, `gate/index.json` | The run: a row per scenario, and the env (host, CPU, cgo, MIME table) |
| `gate/<scenario>/report.md`, `report.json` | Checks with their evidence, metrics and findings |
| `gate/<scenario>/warmup-1/`, `rep-1/`, … | Each rep's `rep.json` and `child.log`, its profiles, `goroutines.txt` after a hard timeout, and `failure-N.txt` at a timed-out read |

`go run -tags disable_libutp ./cmd/streamlab explain <check | known issue | scenario>`, run in `momoshtrem/`, says what a name means.

### When it fails
A failure is a finding about the streaming path or the lab. It's never a reason to edit the baseline.
- `baseline.json` changes only through `streamlab baseline update --reason`, and its digest refuses a hand edit.
- The registry changes only in the PR that fixes the issue (PLAN D6).
- A race, a hang or an unexplained flake gets its own `streaming` issue.
- To reproduce on the Mac: `go run -tags disable_libutp ./cmd/streamlab gate --scenario <name> --reps 3`.

### Non-blocking for now
The job has `continue-on-error: true`, since existing races may surface.
- A failure shows as a red `streamlab` check, but the workflow still passes.
- `publish` waits only for `go` and `web`.
- `streamlab` isn't a required check.

### Making it required
Make it required once `streamlab` has been green on `main` for a week. Check each `main` run's `streamlab` job, since a run passes whatever that job did:
```bash
gh run list --workflow ci.yml --branch main --event push --limit 40 --json databaseId --jq '.[].databaseId' |
  while read -r id; do
    gh run view "$id" --json createdAt,headSha,jobs \
      --jq '[.createdAt[:10], .headSha[:7], ([.jobs[] | select(.name == "streamlab") | .conclusion][0] // "none")] | @tsv'
  done
```
Then, in one PR:
1. Remove the job's `continue-on-error: true`. The affected step keeps its own: it stays a warning for the reviewer.
2. Add `streamlab` to the required checks (§5.2). A PR that doesn't touch the streaming path skips the job, and GitHub counts a skipped job as passed. So it holds only the PRs it runs on.
