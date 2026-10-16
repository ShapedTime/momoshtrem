# Deploying momoshtrem

> Runbook · 2026-10-15 · #37, #38, #42 · Epic #32 · Plan: [`PLAN.md`](PLAN.md) §6, §8
> How a commit goes live on the VM with `ops/deploy/deploy.sh`, how to roll back, and the first deploy on a fresh VM from a copy of the old state.

The stack runs from `/opt/momoshtrem` on the VM (HOST.md §4). Three files there describe the deployed commit, and nothing else does (HOST.md §5):
- `compose.yml` and `Caddyfile`, taken from the deployed commit;
- `.env`, managed by hand (PLAN D6). Its `MOMOSHTREM_TAG` is the deployed commit's full SHA.

The VM never builds. compose pulls `ghcr.io/shapedtime/momoshtrem:<sha>` and `momoshtrem-web:<sha>` (CI.md §2). `config.yaml` lives in `/mnt/data/momoshtrem/config`.

## 1. Deploy

Run the script on the Mac, from the repo. GitHub never reaches the VM (PLAN D3): the script reads `main` with `git fetch`, checks GHCR anonymously, and does the rest over SSH.

```bash
ops/deploy/deploy.sh -n momoshtrem-v2            # dry run: the plan, and a preflight on the VM
ops/deploy/deploy.sh momoshtrem-v2               # deploy the newest green main commit
ops/deploy/deploy.sh momoshtrem-v2 <commit>      # deploy a given main commit, such as a rollback
```

- **The VM.** The first argument is any SSH destination, so the same script deploys to another VM. It needs Docker, the `agha` user in the `docker` group, and `/opt/momoshtrem/.env`: the host playbook and §3 set these up.
- **Options.** `-y` skips the question. `--timeout SEC` sets how long to wait for healthy (default 300).
- **The target.** Without a commit, the script takes the newest `main` commit whose two images are on GHCR: a green commit (CI.md §1). It names any newer commit it skipped. A given commit must be on `main` and have images.

**The plan.** Before changing anything, the script prints:
- what's running, the target, and each service's health;
- the commits added (an upgrade) or removed (a rollback);
- the migrations crossed;
- whether `compose.yml` and the `Caddyfile` change, and whether the VM's copies were edited by hand;
- the `.env.example` diff.

`.env` is managed by hand, so add any new variable to it before you say yes: `ssh momoshtrem-v2`, then edit `/opt/momoshtrem/.env` there. Never print it or paste it anywhere.

**The AI features** (v2 #9) need two of them:
- `ANTHROPIC_API_KEY` turns on the personal picks, Ask momoshtrem and the episode-mapping AI. Without it they stay off.
- `TZ` sets momoshtrem's zone, an IANA name such as `TZ=Europe/Berlin`. compose passes `UTC` when it's unset.
  - With `TZ` set, the nightly picks run at 03:00 local time, not 03:00 UTC.
  - Ask's daily limit and the AI budget's month follow it too.
  - It's read at startup, so the deploy that adds it applies it.

The script then asks before deploying. If the target is already running, it stops.

**On the VM.** One SSH session does the steps below while holding `/run/lock/momoshtrem.lock`. The backups (#35) take the same lock, so the two never overlap.

| Step | If it fails |
|---|---|
| 1. Check that the target's `compose.yml` accepts `.env` (`docker compose config --quiet`) | Stops. Nothing has changed |
| 2. Dump the database to `dumps/<time>-<running sha>.dump`, keeping the newest 10. Skipped when Postgres isn't running, as on a fresh VM | Stops. Nothing has changed |
| 3. Pull the target's images | Stops. Nothing has changed |
| 4. Install the target's `compose.yml` and `Caddyfile`, and set `MOMOSHTREM_TAG` in `.env` | |
| 5. `docker compose up -d --remove-orphans` | **Failed** |
| 6. Wait until every service is healthy | **Failed** |
| 7. Reload Caddy if the `Caddyfile` changed, because Caddy doesn't watch it | **Failed**. Caddy keeps its old config |
| 8. Check that `/api/status` reports the target | **Failed** |
| 9. Record the deploy in `deploy-state` and `deploy.log`. Prune images, keeping those the containers use and ours at the previous commit | |

**Failed** means the target is installed but isn't healthy.
- The script doesn't roll back.
- It records the commit as `last_failed` and prints the last 30 log lines of each unhealthy service.
- It prints the command to roll back.

Fix the cause and deploy again, or roll back (§2).

**The record**, in `/opt/momoshtrem`:

```bash
ssh momoshtrem-v2 cat /opt/momoshtrem/deploy-state   # current, previous and last_failed commits
ssh momoshtrem-v2 cat /opt/momoshtrem/deploy.log     # one line per deploy
```

## 2. Roll back

A rollback is a deploy of an older commit. `deploy-state` names the previous one, and its images are still on the VM:

```bash
ops/deploy/deploy.sh momoshtrem-v2 <previous sha>
```

**Migrations only go forward** (PLAN §6). The plan says whether one lies between the two commits.
- **Without data.** The older code runs on the newer schema and only logs a warning.
- **With the data too.** First restore the dump taken before the newer deploy: it's named after the commit that was running then. Then deploy the older commit.

On the VM, in `/opt/momoshtrem`:

```bash
docker compose stop momoshtrem web
docker compose exec -T postgres dropdb -U momoshtrem momoshtrem
docker compose exec -T postgres createdb -U momoshtrem momoshtrem
docker compose exec -T postgres pg_restore -U momoshtrem -d momoshtrem --no-owner --exit-on-error < dumps/<time>-<older sha>.dump
```

Then, on the Mac, run `ops/deploy/deploy.sh momoshtrem-v2 <older sha>`.

## 3. First deploy on a fresh VM, from a copy of the old state

The VM is built from [`VM.md`](VM.md) and converged by the playbook ([`HOST.md`](HOST.md)). The copy holds the PLAN §5 state, for example in `~/restore/` on the VM:
- `library.dump` (`pg_dump -Fc`), with `row-counts.sql` and the counts taken at the same time;
- Jackett's `/config`;
- the downloaded subtitles;
- the `caddy_data` volume;
- the old `config.yaml`.

Don't copy the torrent cache, `vfs_tree.gob` or the legacy SQLite file: they rebuild themselves or are retired (PLAN §5).

**The order matters.** momoshtrem runs its migrations when it starts, and on an empty database they create the schema, which a restore then collides with. So restore Postgres before momoshtrem starts for the first time.

In `/opt/momoshtrem`:

1. **`.env`.** Write it from `.env.example`, with `chmod 600`:
   - the data paths from HOST.md §4, and `MOMOSHTREM_TAG`;
   - `JACKETT_API_KEY` must match the key in the restored `Jackett/ServerConfig.json`;
   - keep `WEBDAV_USER` and `WEBDAV_PASSWORD_HASH`, or Infuse needs new credentials;
   - `ANTHROPIC_API_KEY` for the AI features, and `TZ` so the 03:00 picks run is local time (§1).
2. **`compose.yml` and `Caddyfile`** from the target commit. Postgres has to start alone first, so this deploy is by hand. On the Mac:
   ```bash
   SHA=<full sha>
   for f in compose.yml Caddyfile; do
     git show "$SHA:$f" | ssh momoshtrem-v2 "cat > /opt/momoshtrem/$f"
   done
   ```
   Then, on the VM, run `docker compose pull`.
3. **Postgres alone**, then the restore and the count check:
   ```bash
   docker compose up -d postgres            # wait for (healthy) in docker compose ps
   docker compose exec -T postgres pg_restore -U momoshtrem -d momoshtrem --no-owner --exit-on-error < ~/restore/library.dump
   docker compose exec -T postgres psql -U momoshtrem -d momoshtrem -At -F' ' < ~/restore/row-counts.sql | diff ~/restore/row-counts.txt - && echo identical
   ```
   `--no-owner` gives every object to the `momoshtrem` role, whatever owned it before.
4. **Jackett.** Unpack its `/config` into `/mnt/data/jackett`, so that `/mnt/data/jackett/Jackett/ServerConfig.json` exists. Check the layout first with `tar -tf`. The folder stays owned by `agha` (`PUID=1000`). That file also carries the FlareSolverr URL (#106); a backup older than that needs it set once (README › Configure Jackett).
5. **Subtitles.** Unpack them into `/mnt/data/momoshtrem/data/data/subtitles`, as root: the container runs as root. The DB rows point at `movie/<id>/…` and `episode/<id>/…` under it.
6. **`caddy_data`.** The pinned project name makes the volume `momoshtrem_caddy_data`. A throwaway container creates it and unpacks the old volume into it:
   ```bash
   docker compose run --rm --no-deps -T caddy tar -xf - -C /data < ~/restore/caddy_data.tar
   docker compose run --rm --no-deps caddy ls /data/caddy/pki/authorities/local   # root.crt and intermediate.crt: the old internal CA (unused since #19)
   ```
7. **`config.yaml`** in `/mnt/data/momoshtrem/config`. Keep only what env doesn't cover and what differs from the defaults. Secrets go in `.env`. Set the cache cap from VM.md §1:
   ```yaml
   torrent:
     global_cache_size: 53760   # MiB: at most 75% of the data disk (VM.md §1)
   metrics:
     enabled: true              # on 127.0.0.1:9090 (PLAN D8)
   ```
8. **Start everything** with `docker compose up -d`, and wait until `docker compose ps` shows every service healthy. The momoshtrem log shows each migration it applied (`docker compose logs momoshtrem | grep migration`).
9. **Backfill** the TMDB metadata once (v2 PLAN H3). It runs beside the server:
   ```bash
   docker compose run --rm momoshtrem backfill-metadata --dry-run   # read the summary first
   docker compose run --rm momoshtrem backfill-metadata
   ```
10. **Check** the web UI, and Infuse at home and away. `ops/deploy/deploy.sh -n momoshtrem-v2` should show the commit as running. From then on, deploys go through §1.
