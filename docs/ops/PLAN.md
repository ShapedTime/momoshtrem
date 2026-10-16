# momoshtrem ops: reproducible server, CI/CD and backups

> Status: agreed plan · 2026-10-14, updated 2026-10-15 (D3 and §8 by #38) · Epic #32
> A separate track from v2 (`docs/redesign/PLAN.md`). Issues are labelled `ops`.

## 0. How to use this plan

- The work is tracked in the GitHub issues #33–#38 and #42 under epic #32, using the normal workflow in `CLAUDE.md`: claim, branch `<issue#>-<slug>`, and a PR with `Closes #n`.
- This plan fixes **decisions and boundaries**, not implementations. Each issue's agent designs its own approach within them and writes the runbook for its part in `docs/ops/`.
- If an implementation has to break a decision here, update this file in the same PR and explain why on the issue.
- **Keep it small.** The whole setup is one VM, Docker Compose, a few systemd timers and one Ansible playbook. If a step seems to need a new service or platform, stop and ask.
- **The repo is public.** No secrets, no public hostname and no personal data in commits, workflows or logs.
- When an issue needs the user (Proxmox snapshot, router, accounts, the password manager), add `blocked` and say exactly what's needed.

## 1. Goals and non-goals

**Goals**
1. **Reproducible.** The server can be rebuilt from the repo, a few password-manager entries and the latest backup.
2. **CI/CD.** PRs are checked, and every commit on `main` produces versioned images. Deploying is one command on the Mac, and rolling back is the same command with an older version.
3. **Backups.** They run nightly, are encrypted and stored off-site, alert on failure, and have been proven by a restore.

**Non-goals (this round)**
- HTTPS certificates and the `:3000` exposure. These belong to #19 in the v2 track.
- Remote streaming speed.
- A staging environment.
- Creating the VM with IaC.
- HA or more than one host.
- A metrics stack (Prometheus or Grafana).

## 2. Decisions

| # | Decision | Notes |
|---|---|---|
| D1 | Backups go to **Backblaze B2**, encrypted on the VM | Nightly, plus before every deploy. restic is the recommended tool. |
| D2 | Alerts go through **Healthchecks.io** | The backup and heartbeat jobs ping it, and it notifies the user when a ping fails or doesn't arrive. Nothing extra runs on the VM. A deploy runs in a terminal, so it reports there. |
| D3 | **Deploys run from the Mac over SSH; GitHub never reaches the VM** | CI checks PRs and publishes images for every `main` commit. `ops/deploy/deploy.sh` deploys a chosen commit (§8). The repo is public, so no workflow can touch the VM and GitHub holds no deploy secret: no Deploy workflow, no self-hosted runner (fork PRs could run code on it), no inbound ports. Changed on 2026-10-15 (#38), replacing a GitHub Deploy button and a pull timer on the VM. |
| D4 | **Ansible** configures the VM, run from the Mac | The VM itself is created by hand in Proxmox from a documented spec. The Proxmox host is out of scope. |
| D5 | **Build a fresh VM and cut over** | Reversed on 2026-10-15 (#42): the as-is inventory found too much to fix in place. The new VM is created by hand from [`VM.md`](VM.md), configured by Ansible and deployed from scratch, and the data is copied across. It keeps its own IP, 192.168.0.99: at cutover the router forwards (36911, and 443 since #19) move to it, and the old VM keeps the .81 reservation. The old VM stays, stopped, as the way back until the new one is proven. The user takes Proxmox snapshots by hand before risky steps. |
| D6 | **Secrets live only on the VM** and in the encrypted backup | `.env` is managed by hand on the VM. Secrets never go in the repo or in GitHub, and there is no Ansible Vault. The break-glass values the VM can't hold live in the user's password manager (§7). |
| D7 | **Images live on GHCR, tagged by the full commit SHA** | The images are public, since the code is public anyway. Tags are immutable, and the VM never builds. |
| D8 | Keep **pgAdmin** (LAN only). **Don't publish Jackett or metrics to the LAN** | compose publishes them on the VM's loopback only (`127.0.0.1:9117` and `127.0.0.1:9090`, and the web app's `127.0.0.1:3000` since #19), so an SSH tunnel reaches them: `ssh -L 9117:localhost:9117 momoshtrem-v2`. FlareSolverr (#106) isn't published at all: only Jackett calls it, at `http://flaresolverr:8191` on the compose network. |
| D9 | **Go-live happens at cutover** | The first deploy is current `main` (v2). The old VM went down before #35 and #38 existed (§9), so it's deployed by hand and protected by the #33 copy and a Proxmox snapshot. Later deploys use `ops/deploy/deploy.sh`, behind its pre-deploy dump and the B2 backup (#35). |

## 3. Current state: findings that drive this plan

Taken read-only on 2026-10-14. #33 records the full setup as it is today in `docs/ops/AS-IS.md`.

**Host.** `momoshtrem-vm` is a Proxmox KVM guest running Ubuntu 24.04, with 4 vCPU and 8 GB RAM, behind the home NAT. Remote access is through a DuckDNS name and a router port forward to Caddy.
- **The root filesystem (a 15 GB LV) is 94% full.** Containerd images take 6.3 GB, including two `golang` images left from building on the box. Build cache takes 1.6 GB and journald 1.3 GB. The volume group has about 14 GB unallocated.
- **The data disk** is 120 GB at `/mnt/data`, 81% used.
  - `/mnt/data/3/momoshtrem/` holds `pgdata` (67 MB), `config/config.yaml` and `data/data/`.
  - `data/data/` holds the torrent cache (90 GB), `subtitles/`, `cache/vfs_tree.gob` and a legacy SQLite file.
  - Jackett's config is in `/mnt/data/3/jackett`.
- **There are no backups of any kind**: no cron, no timers, nothing off the box. qemu-guest-agent isn't running.
- **ufw allows** 3000, 4444, 9117, 5050, 9090, 80, 443 and 36911 from anywhere. Docker-published ports bypass ufw, though, so compose is what actually decides what's exposed.
- **A systemd unit** (`streaming-stack.service`) is meant to run `docker compose up -d` at boot, but it points at the missing `/opt/streaming-stack` and fails. The stack comes back only through `restart: unless-stopped`.

**App checkout.** It lives at `/opt/streaming-stack-3/apple-tv-tor-stream`.
- **It is stale.** It is at `e063821` (2026-04-23), 23 commits behind `main`, and still runs the old `PrettyTVCatalog` frontend.
- **Images are built on the VM** with `docker compose build`.
- **The data paths depend on an untracked file.** `compose.override.yml` maps them to `/mnt/data/3/...`. Without it, compose falls back to `./data/...`, where a stray root-owned `./data/pgdata` holds an empty 47 MB cluster, created by accident on 2026-04-19. Losing the override would silently start the app on an empty database.
- **`.env` and the `Caddyfile` exist only on the VM** (both are gitignored). The Caddyfile still proxies to `prettytvcatalog:3000`. A git stash named "compose" is left over.
- **The compose project name comes from the folder name**, and so does the name of the `caddy_data` volume. That volume holds Caddy's internal CA, which Infuse trusts.

**Repo.**
- **There's no CI.** `.github/` doesn't exist and `.gitignore` excludes it. There is no registry, no tags and no image names.
- **`momoshtrem/` has no `.dockerignore`**, so local torrent data ends up in the build context.
- **Jackett and pgAdmin images use `latest`.**
- **momoshtrem has no healthcheck.** `/api/status` requires the backend token, so it can't serve as one. The build already stamps a version (`-X main.version`), which `/api/status` reports.
- **Migrations are embedded and run at startup, forward only.** An old binary on a newer schema only logs a warning.
- **Moving from `e063821` to `main`** means:
  - the `web` service replaces `prettytvcatalog`;
  - new env vars: `BACKEND_API_TOKEN`, `SECRET_KEY`, `APP_USERNAME`, `TRUSTED_PROXY_HOPS`, the Trakt and Anthropic keys, and `TZ` (the AI features' zone; DEPLOY.md §1);
  - migrations 002, 003 and 007, followed by `backfill-metadata` (v2 PLAN H3).

## 4. Target shape

```
 PR ───────────► CI checks (Go vet/test, web lint/tsc/jest, docker build)
 merge to main ─► CI builds images ─► GHCR  ghcr.io/shapedtime/momoshtrem:<sha>
                                            ghcr.io/shapedtime/momoshtrem-web:<sha>

 Mac ── ops/deploy/deploy.sh <host> [<sha>] ── SSH ──► momoshtrem VM
          picks a green main commit,            lock ─► pre-deploy dump ─► pull <sha>
          prints the plan, asks                 ─► compose up ─► wait healthy ─► record

 momoshtrem VM
   backup timer:  lock ─► backup ─► B2 ─► ping Healthchecks.io      (nightly)
   heartbeat:     every container healthy ─► ping Healthchecks.io

 Mac ── ansible-playbook ──► momoshtrem VM   (OS, Docker, folders, timers)
```

| Owner | Owns |
|---|---|
| **Repo** | The compose file, the Caddyfile (host values from env), the Ansible playbook, the deploy and backup scripts, and the docs |
| **Ansible** | OS packages, Docker, log limits, the disk, the host firewall, the guest agent, folders and the timers |
| **VM only** | `.env` and the data |
| **B2** | Encrypted copies of the VM-only things |
| **Password manager** | The few values needed to reach B2 once the VM is gone (§7) |

## 5. State inventory

| State | Where today | Class | Notes |
|---|---|---|---|
| Postgres library | `pgdata` | **Critical** | Back it up as a logical dump (`pg_dump`), never as the raw data directory. |
| `.env` | checkout root | **Critical** | Holds all the secrets (D6), and exists only here and in the backup. |
| Jackett `/config` | `/mnt/data/3/jackett` | **Critical** | The indexer setup, and the FlareSolverr URL and max timeout (`ServerConfig.json`, #106). Its API key must match `JACKETT_API_KEY`, so restore the two together. |
| momoshtrem `config.yaml` | `.../momoshtrem/config` | Back up | Tuning: cache size, ports, metrics. |
| Downloaded subtitles | `.../data/data/subtitles` | Back up | DB rows point at these files, and downloading them again uses up OpenSubtitles quota. |
| `caddy_data` volume | `momoshtrem_caddy_data`, a Docker named volume | Back up | The Let's Encrypt account and certificate (#19), and the old internal CA, now unused. If it's lost, Caddy gets a new certificate when it starts; nothing has to be trusted again. The pinned project name fixes the volume's name (#37). |
| Deploy state | `/opt/momoshtrem/deploy-state` and `deploy.log` (#38) | Back up | The current, previous and last-failed commit, and one line per deploy. |
| Pre-deploy dumps | `/opt/momoshtrem/dumps` (#38) | Excluded | The newest 10 `pg_dump`s, one per deploy, for a rollback across a migration. The nightly backup covers the same data. |
| Caddyfile | the repo (#37) | Tracked | Its host values and the WebDAV hash come from `.env`. |
| Torrent cache | `.../data/data/torrents` (90 GB) | Excluded | Rebuilds itself. Never move it; delete it if the space is needed. |
| `vfs_tree.gob` | `.../data/data/cache` | Excluded | Rebuilt from the DB. |
| Legacy SQLite file | `.../data/data/momoshtrem.db` | Excluded | Archived once (#33). It isn't copied to the new VM (#42). |

**Defaults** (the implementing agent may change them, with a reason):
- Backups run nightly at a quiet hour.
- Keep 7 daily, 4 weekly and 6 monthly backups. Pre-deploy backups follow the same policy.
- The B2 bucket is private and its key is scoped to that one bucket.
- B2 keeps prior versions for a few weeks, so a compromised VM can't erase the history.

## 6. Rollback model

From least to most effort:

| Layer | When | How |
|---|---|---|
| **App** | A deploy misbehaves | Deploy the previous commit: `ops/deploy/deploy.sh <host> <sha>`. Images are immutable, and the VM keeps the previous ones. |
| **Data** | The rollback crosses a migration, or the data went bad | Also restore the dump that deploy took beforehand. Migrations only go forward, and an old binary on a newer schema only warns. |
| **Machine** | An OS or Ansible change broke the host | Revert to the Proxmox snapshot taken before the change (D5). Without one, rebuild as in the next row. |
| **Disaster** | The VM or its disk is gone | Create a VM from [`VM.md`](VM.md), run Ansible ([`HOST.md`](HOST.md)), restore from B2 (including `.env`), then deploy the last good commit in the order of [`DEPLOY.md`](DEPLOY.md) §3. #42 does this once with the old VM's data. |

At cutover (#42), the old VM is the only way back to the old version, because `e063821` has no images on GHCR. It stays stopped, not deleted, until the new VM has three green nightly backups. Rolling back means shutting the new VM down, pointing the router forwards back at .81 and starting the old VM; anything written on the new VM in between is lost.

## 7. Secrets and break-glass

- **`.env` on the VM is the only source of app secrets** (D6). `.env.example` lists every variable, with a comment saying what it is and where to regenerate it: TMDB, Jackett, OpenSubtitles, Trakt, Anthropic, or `openssl rand`.
  - `POSTGRES_PASSWORD` only takes effect when the database is first created.
  - `SECRET_KEY` encrypts the stored Trakt tokens, so it has to travel with the DB.
- **The password manager holds what the VM can't**: the backup repository password, the B2 key ID and secret, the bucket name, and the Healthchecks.io login. With these plus the repo, a rebuild can start from nothing.
- **Ansible asks for the sudo password at run time.** It is never stored in the repo.
  - An agent can't answer the prompt. When an agent runs the playbook, the user may put the password in a gitignored file for that session only, and delete it afterwards ([`HOST.md`](HOST.md) §2).
- **GitHub holds no deploy secrets.** CI pushes images with the built-in token, and the VM only pulls the public images. Deploys use the Mac's SSH key (D3).
- **Secret rotation is out of scope for now.** The comments in `.env.example` are enough to rotate a secret by hand.

## 8. Deploys

`ops/deploy/deploy.sh` (#38) meets this contract. It runs on the Mac and reaches the VM over SSH. Runbook: [`DEPLOY.md`](DEPLOY.md).

- **The target** is a full `main` commit SHA. It's either given on the command line or, by default, the newest `main` commit whose two images are on GHCR.
  - The script refuses a commit that isn't on `main` or has no images.
  - GitHub's only part is hosting the repo and the images (D3).
- **Compose matches the images.** The compose file and Caddyfile come from the same commit as the images. The VM never builds.
- **The plan comes first.** Before changing anything, the script prints:
  - the commits added or removed;
  - the migrations crossed;
  - whether the compose file and Caddyfile change;
  - the `.env.example` diff.

  Then it asks. `-n` stops after the plan and a preflight on the VM.
- **Idempotent.** `/opt/momoshtrem/deploy-state` records the current, previous and last-failed commit. If the target is already current, the script does nothing.
- **One thing at a time.** Deploy and backup share one lock, `/run/lock/momoshtrem.lock` (`flock`). A deploy refuses to start while the lock is held.
- **Order:**
  1. Take the lock.
  2. Preflight: the target's compose file accepts `.env`.
  3. Dump the database into `/opt/momoshtrem/dumps`. Stop if that fails.
  4. Pull the images. Stop if that fails: nothing has changed yet.
  5. Install the compose file, the Caddyfile and `MOMOSHTREM_TAG`, then run `up`.
  6. Wait until every service is healthy, with a timeout. Reload Caddy if its file changed.
  7. Check that `/api/status` reports the commit, then record the result.
- **On failure:** no automatic rollback.
  - The stack is left as it is, and the commit is recorded as last failed.
  - The script prints the unhealthy services' logs and the rollback command.
  - A human decides what happens next.
- **Image retention.** Keep the images the containers use, plus ours at the previous commit. Remove older versions of the stack's images.
- **One-time steps stay manual and documented.** Examples are running `backfill-metadata` and deleting `vfs_tree.gob` for v2. The script doesn't run arbitrary hooks.
- **Heartbeat.** A timer on the VM pings Healthchecks.io when every container is healthy, so both a dead VM and a broken stack raise an alert. It's installed with #35's backup timer.

## 9. Workstreams and waves

| Issue | Task | Depends on |
|---|---|---|
| #33 | **0 · Safety net and as-is inventory**: take a Proxmox snapshot, copy everything in §5 to the Mac, and document today's setup | (none); blocks #34 |
| #34 | **A · Ansible baseline** for the new VM: packages, Docker log limits, journald cap, guest agent, host firewall, folders, boot unit | #33, the VM (#42) |
| #35 | **B · Backups to B2**: nightly encrypted backups, retention, alerts, the heartbeat (§8), and the restore runbook and drill. It takes #38's lock | #34 |
| #36 | **C · CI**: PR checks, images on GHCR by SHA, `.dockerignore`, Dependabot | (none) |
| #37 | **D · Production compose**: GHCR images, pinned versions, required paths, pinned project name, fewer published ports, tracked Caddyfile, liveness endpoint and healthchecks | #36 |
| #38 | **E · Deploy script** run from the Mac (§8), plus the deploy and rollback runbook | #34, #37 |
| #42 | **F · New VM and cutover**: build the VM from the spec, deploy `main` on it, copy the data, cut over, practise a rollback, retire the old VM, finish the runbook. Supersedes #39 | #34, #35, #37, #38 |

```
0 (#33) ──► A (#34) ──┬──────────► B (#35) ──┐
                      ▼                      ├──► F (#42)
C (#36) ──► D (#37) ──► E (#38) ─────────────┘
```

The waves:
1. **#33 and #36** can start now, in parallel.
2. **#34 and #37** come next.
3. **#35 and #38** follow. #38 no longer waits for #35: the backup job takes its lock.
5. **#42** is last. #39 (converge the old VM) is superseded by it.

The old VM was shut down early, on 2026-10-15. So #42 deployed by hand ([`DEPLOY.md`](DEPLOY.md) §3) with #37 folded in, before #35 and #38 existed. Later deploys use #38's script. Until #35 lands, Proxmox snapshots and the #33 copy on the Mac are the safety net.

## 10. Verification

Epic #32 is done when all of these hold:

1. **Reproducible.** A check-mode Ansible run against the live VM shows no changes, and the VM spec plus the runbook describe a rebuild from nothing.
2. **CI/CD.** A PR shows green checks, and a merge publishes images. `ops/deploy/deploy.sh` puts that commit live, and `/api/status` reports it.
3. **Rollback.** A rollback between two commits has been practised once with the script.
4. **Backups.**
   - Three nightly runs in a row are green.
   - A forced failure raised an alert.
   - One restore from B2 has been checked (DB row counts and files).
5. **The product still works.** Infuse plays at home and remotely, and the web UI works.

## 11. Risks and deferred items

**Risks**
- **Missing state in the move to a new VM.** Anything the copy misses is gone once the old VM is deleted.
  - Mitigation: the as-is inventory (#33) lists the state, the restore is rehearsed on the Mac with the real dump first, and the old VM is kept until the new one has three green nightly backups.
- **The v2 jump at cutover.** Several migrations and a backfill all happen in one deploy.
  - Mitigation: a rehearsal of the restore and migrations on the Mac first, a snapshot, and the H3 smoke test. The old VM is the way back (§6).
- **Losing `caddy_data`.** Since #19 it holds a Let's Encrypt certificate, so a new volume only means a new certificate. Let's Encrypt allows 5 a week for the same name.
  - Mitigation: #37 pins the project name, so the volume is always `momoshtrem_caddy_data`, and #35 backs it up.
- **Disk pressure.** The old VM's root disk filled up, and the new data disk is 70 GiB until cutover.
  - Mitigation: the new root LV fills its disk and the VM never builds (VM.md §1). Image retention (§8) keeps it that way. The torrent cache cap is 52 GiB until the data disk grows to 120 GiB.
- **One person, one VM.** Accepted. The runbook is the mitigation.

**Deferred.** Each of these gets its own issue when it's needed.
- **Postgres major upgrade.** Stay on 16, and upgrade by dump and restore when the time comes.
- **GHCR image cleanup.** Public images cost nothing to store.
- **Tooling for secret rotation.**
- **Automatic rollback.**
- **Security review of the new VM**, including SSH root login (#43).
- **The doubled `/data/data` path.** It comes from relative defaults combined with `WORKDIR /data`. It's harmless and documented.
- **Remote streaming speed.** It needs measuring first: home upload, the torrent swarm, the proxy.
