# CI and images

> Runbook · 2026-10-15 · #36 · Epic #32 · Plan: [`PLAN.md`](PLAN.md) D7, §4
> `.github/workflows/ci.yml` checks every PR and publishes images for every `main` commit. The VM never builds.

## 1. What runs when

| Job | On | Does |
|---|---|---|
| `go` | PRs and `main` | `go vet ./...` and `go test ./...` in `momoshtrem/`. A throwaway Postgres 16 service is set as `MOMOSHTREM_TEST_DATABASE_URL`, so the integration tests run instead of skipping |
| `web` | PRs and `main` | `npm ci`, `npm run lint`, `npx tsc --noEmit` and `npm test` in `web/`. Node is the major in `web/Dockerfile`, so CI tests what ships |
| `image (momoshtrem)`, `image (momoshtrem-web)` | PRs | Build each image for `linux/amd64` without pushing. They run alongside the checks |
| `publish (momoshtrem)`, `publish (momoshtrem-web)` | `main` | Wait for `go` and `web`, then build and push the image (§2) |

The commands are the ones in `CLAUDE.md` "Commands". A new push to a PR cancels that PR's older run. `main` runs are never cancelled, so every commit gets its images.

**A green `main` commit** is one whose `publish` jobs succeeded. Its images exist, and its checks passed on that exact commit. The Deploy button (#38) offers only these.

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
- **The third-party images in `compose.yml`**: Jackett, Postgres, pgAdmin and Caddy, pinned to exact tags (#37). Our own two images are ignored: deploys move their tag.

## 5. One-time setup

The user does these steps in GitHub. Agents can't.

1. **Make the packages public**, after the first `publish` run:
   - GHCR creates new packages as private, and there's no API to change it.
   - For each of `momoshtrem` and `momoshtrem-web`, go to your profile → **Packages** → the package → **Package settings** → **Danger Zone** → **Change visibility** → **Public**.
   - This can't be undone, which is fine: the code is public too.
2. **Require the checks on `main`**:
   - Go to **Settings** → **Rules** → a branch ruleset on the default branch → **Require status checks to pass**.
   - Add `go`, `web`, `image (momoshtrem)` and `image (momoshtrem-web)`.
3. **Approve outside contributors' PR runs**: **Settings** → **Actions** → **General** → **Require approval for all external contributors**.

## 6. Changing CI

- **Keep PR checks fast.** v2 agents open PRs often.
  - Playwright E2E can come later as its own job: `npx playwright install --with-deps chromium`, then `npx playwright test`.
  - Add it to the required checks only once it's stable.
- **Renaming a job renames its check.** Update the required checks (§5.2) in the same change, or PRs will wait forever on a check that no longer runs.
- **Pin new actions by SHA**: `gh api repos/<owner>/<repo>/commits/<tag> --jq .sha`.
