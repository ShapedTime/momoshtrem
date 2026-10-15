# Deploying momoshtrem

> Runbook · 2026-10-15 · #37, #42 · Epic #32 · Plan: [`PLAN.md`](PLAN.md) §6, §8
> How a commit goes live on the VM by hand, until the deploy agent (#38) does it, and the first deploy on a fresh VM from a copy of the old state.

The stack runs from `/opt/momoshtrem` on the VM (HOST.md §4). Three files there describe the deployed commit, and nothing else does (HOST.md §5):
- `compose.yml` and `Caddyfile`, taken from the deployed commit;
- `.env`, managed by hand (PLAN D6). Its `MOMOSHTREM_TAG` is the deployed commit's full SHA.

The VM never builds. compose pulls `ghcr.io/shapedtime/momoshtrem:<sha>` and `momoshtrem-web:<sha>` (CI.md §2). `config.yaml` lives in `/mnt/data/momoshtrem/config`.

## 1. Manual deploy, until #38

Pick a green `main` commit, one whose `publish` jobs passed (CI.md §1), and use its full SHA.

**On the Mac**, from the repo, copy that commit's compose file and Caddyfile across:

```bash
git fetch origin
SHA=<full sha>
for f in compose.yml Caddyfile; do
  git show "$SHA:$f" | ssh momoshtrem-v2 "cat > /opt/momoshtrem/$f"
done
```

**On the VM**, in `/opt/momoshtrem`, straight after:

```bash
SHA=<full sha>
docker compose exec -T postgres pg_dump -U momoshtrem -Fc momoshtrem > ~/pre-deploy-$(date +%F-%H%M).dump   # until #35 does it
sed -i "s/^MOMOSHTREM_TAG=.*/MOMOSHTREM_TAG=$SHA/" .env
docker compose pull      # if this fails, nothing has changed yet
docker compose up -d
docker compose ps        # repeat until every service shows (healthy)
```

**Check the commit** that `/api/status` reports. The token is read inside the container, so it never reaches the shell:

```bash
docker compose exec momoshtrem sh -c 'wget -qO- --header "X-Momoshtrem-Token: $BACKEND_API_TOKEN" http://127.0.0.1:4444/api/status' | jq -r .version
```

**Rollback** is the same steps with an older SHA. Migrations only go forward (PLAN §6), so check whether one lies between the two commits:

```bash
git diff --stat <old> <new> -- momoshtrem/internal/library/migrations/
```

If one does, restore the dump taken before the newer deploy, between `pull` and `up -d`:

```bash
docker compose stop momoshtrem web
docker compose exec -T postgres dropdb -U momoshtrem momoshtrem
docker compose exec -T postgres createdb -U momoshtrem momoshtrem
docker compose exec -T postgres pg_restore -U momoshtrem -d momoshtrem --no-owner --exit-on-error < ~/pre-deploy-<when>.dump
```

## 2. First deploy on a fresh VM, from a copy of the old state

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
   - keep `WEBDAV_USER` and `WEBDAV_PASSWORD_HASH`, or Infuse needs new credentials.
2. **`compose.yml` and `Caddyfile`** from the target commit (§1), then `docker compose pull`.
3. **Postgres alone**, then the restore and the count check:
   ```bash
   docker compose up -d postgres            # wait for (healthy) in docker compose ps
   docker compose exec -T postgres pg_restore -U momoshtrem -d momoshtrem --no-owner --exit-on-error < ~/restore/library.dump
   docker compose exec -T postgres psql -U momoshtrem -d momoshtrem -At -F' ' < ~/restore/row-counts.sql | diff ~/restore/row-counts.txt - && echo identical
   ```
   `--no-owner` gives every object to the `momoshtrem` role, whatever owned it before.
4. **Jackett.** Unpack its `/config` into `/mnt/data/jackett`, so that `/mnt/data/jackett/Jackett/ServerConfig.json` exists. Check the layout first with `tar -tf`. The folder stays owned by `agha` (`PUID=1000`).
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
10. **Check** the commit as in §1, the web UI, and Infuse at home and away.
