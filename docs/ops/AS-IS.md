# momoshtrem-vm as it is today

> Recorded read-only on 2026-10-14 · #33 · Epic #32 · Plan: [`PLAN.md`](PLAN.md)
> A Proxmox snapshot was taken the same day, before any ops change. Writing this changed nothing on the VM.

This is how the server ran before #34–#39 started changing it. It holds no secrets and no public hostname. The DuckDNS name is written `<duckdns-host>` throughout.

## 1. Findings that matter

- **The boot unit is broken.** `streaming-stack.service` points at a folder that no longer exists, and failed at the last boot. The stack comes back only because every container has `restart: unless-stopped` (§11).
- **The data paths depend on an untracked file.** `compose.override.yml` maps every service to `/mnt/data/3/...`. Without it, compose uses `./data/...`, where a stray, empty cluster is waiting (§5).
- **Secrets live in five places**, not just `.env`:
  - `config.yaml` holds the TMDB key and the **only copy of the OpenSubtitles credentials**;
  - the Caddyfile holds the WebDAV basic-auth hash;
  - `caddy_data` holds the CA private key;
  - Jackett's config holds its API key (§10).
- **The router forwards 3000 and 36911.** Port 3000 is the old web UI over **plain HTTP**, published straight from its container, so it is reachable from the internet (§4).
- **Nothing is backed up and nothing is scheduled.** There are no crontabs and no custom timers. The home IP is static, so DuckDNS is never updated (§4).
- **The root disk is 94% full**, and the volume group has 14.5 GB unallocated (§2).

## 2. Host

| | |
|---|---|
| Hypervisor | Proxmox, KVM guest: q35 machine, virtio-scsi disks, virtio-net NIC |
| Hostname | `streaming` (the SSH alias on the Mac is `momoshtrem-vm`) |
| OS | Ubuntu 24.04.3 LTS, kernel 6.8.0-146-generic |
| CPU / RAM | 4 vCPU (host type: Intel N100), 7.7 GiB |
| Time | UTC, NTP active |
| Updates | unattended-upgrades on (`Update-Package-Lists 1`, `Unattended-Upgrade 1`) |
| Guest agent | qemu-guest-agent isn't installed |
| Docker | Engine 29.1.3, Compose v5.0.0, overlayfs, root `/var/lib/docker` |
| Docker daemon config | None: no `/etc/docker/daemon.json` and no systemd drop-in, so the json-file logs have no size cap |
| journald | 1.3 GB, no cap (`journald.conf` is the default) |
| Other services | sysstat (collects every 10 min) and vnstat |

**Disks**

| Device | Size | Use |
|---|---|---|
| `sda1` | 1 GB | `/boot/efi` (vfat) |
| `sda2` | 2 GB | `/boot` (ext4) |
| `sda3` | 28.9 GB | LVM PV of `ubuntu-vg`: one LV `ubuntu-lv` of 14.5 GB for `/` (ext4), with **14.47 GB free in the VG** |
| `sdb` | 120 GB | `/mnt/data` (ext4, no partition table). Mounted in fstab by **`/dev/sdb`, not by UUID** |

Usage on 2026-10-14:
- `/` is 13 GB of 15 GB (94%, 849 MB free).
- `/mnt/data` is 91 GB of 118 GB (81%).

Docker's share:
- images take 6.6 GB, of which 3.5 GB is reclaimable;
- build cache takes 1.6 GB.

## 3. Network and access

- **Interface.** `enp6s18` has 192.168.0.81/24 from **DHCP** (netplan: `dhcp4: true`). It comes from a DHCP reservation on the router. The gateway is 192.168.0.1, and DNS is 192.168.0.82 and 8.8.8.8.
- **Docker networks.** The compose network is `apple-tv-tor-stream_default` (172.18.0.0/16). The unused `docker0` is 172.17.0.0/16.
- **Users.** `agha` is in the `sudo` and `docker` groups. sudo asks for a password, and `/etc/sudoers.d` has only the README.
- **SSH.** Port 22. Key login only: `passwordauthentication no` and `kbdinteractiveauthentication no`.
  - `permitrootlogin without-password` is set, but root's `authorized_keys` is empty.
  - `agha` has one key.
- **Listening on the host.** 22, plus the Docker-published 3000, 5050, 9090, 9117 and 36911, all on IPv4 and IPv6.

**ufw** (active; default: deny incoming, allow outgoing, deny routed; logging low):

| Allowed from anywhere, v4 and v6 |
|---|
| 22/tcp, 36911/tcp, 3000/tcp, 4444/tcp, 9117/tcp, 80/tcp, 80/udp, 443/tcp, 443/udp, 9090/tcp, 5050/tcp |

Docker-published ports bypass ufw, so these rules don't decide what's exposed: compose does. 4444, 80 and 443 aren't published by anything today.

## 4. Router and DuckDNS

- **Port forwards** on the home router (TP-Link), to 192.168.0.81:

  | External | Internal | Reaches |
  |---|---|---|
  | 36911 | 36911 | Caddy: WebDAV over TLS (internal CA) with basic auth, used by Infuse away from home |
  | 3000 | 3000 | `prettytvcatalog` directly, over **plain HTTP**. Caddy isn't involved |

- **DuckDNS.** `<duckdns-host>` points at the home IP, which is static, and was set once by hand in the DuckDNS dashboard. **Nothing updates it**: no cron, timer, container or router client.
  - `DUCKDNS_TOKEN` is in `.env`, but no running container reads it.
  - The `serfriz/caddy-duckdns` image is on the box but unused. Shell history shows DNS-01 TXT records being set by hand. It looks like ACME was tried and then dropped for `tls internal`.

## 5. App checkout

**Path:** `/opt/streaming-stack-3/apple-tv-tor-stream`, owned by `agha`, remote `git@github.com:ShapedTime/momoshtrem.git`.

- **Commit.** `main` is at `e063821` (2026-04-23), 24 commits behind `origin/main` on 2026-10-14. The UI is still the old `PrettyTVCatalog`.
- **Stash.** `stash@{0}` is "On main: compose". It's an old compose edit from the distribyted → momoshtrem switch: it hard-codes `/mnt/data/3/...` paths and the momoshtrem env. Nothing live depends on it.
- **Untracked, VM-only files** (all gitignored):
  - `.env` (mode 600) holds these keys: `APP_PASSWORD`, `TMDB_API_KEY`, `SESSION_SECRET`, `JACKETT_API_KEY`, `DUCKDNS_TOKEN`, `POSTGRES_PASSWORD`, `PGADMIN_EMAIL`, `PGADMIN_PASSWORD`. There's no `OPENSUBTITLES_*` (see §10).
  - `Caddyfile` (§8).
  - `compose.override.yml`, last edited 2026-04-19. It holds no secrets:
    ```yaml
    services:
      prettytvcatalog:
        volumes:
          - /mnt/data/3/prettytv:/app/data
      jackett:
        volumes:
          - /mnt/data/3/jackett:/config
      momoshtrem:
        volumes:
          - /mnt/data/3/momoshtrem/config:/config
          - /mnt/data/3/momoshtrem/data:/data
      postgres:
        volumes:
          - /mnt/data/3/momoshtrem/pgdata:/var/lib/postgresql/data
    ```
- **Stray `./data/`** is owned by root and dates from 2026-04-19, 19:59–20:18.
  - **`pgdata`** is a PG 16 cluster of 47 MB. Its `momoshtrem` database is 7.4 MB, the same size as the empty templates, so it has no library data. It was created when compose ran without the override, just before the override was last saved.
  - **`momoshtrem/`** has only empty folders (28 KB).
  - It's safe to delete at cleanup (#39).
- **Builds happen here.** `docker compose build` from `./momoshtrem` and `./PrettyTVCatalog`. `grafana/dashboards/momoshtrem.json` is tracked, but nothing runs Grafana.

## 6. Compose project

The project name is **`apple-tv-tor-stream`**, taken from the folder name. The config files are `compose.yml` and `compose.override.yml`. Every container has restart policy `unless-stopped`, and all of them were last started at 2026-10-11 16:14 UTC.

| Container | Image | Source | Published | Mounts | Health |
|---|---|---|---|---|---|
| `caddy` | `caddy:2-alpine` | pulled | 36911, 9090, 5050 | `caddy_data:/data`, `caddy_config:/config`, `./Caddyfile:/etc/caddy/Caddyfile:ro` | none |
| `prettytvcatalog` | `apple-tv-tor-stream-prettytvcatalog` | built here | **3000** | `/mnt/data/3/prettytv:/app/data` (empty) | healthy |
| `momoshtrem` | `apple-tv-tor-stream-momoshtrem` | built here | none (4444, 9090, 36911 internal) | `/mnt/data/3/momoshtrem/config:/config`, `/mnt/data/3/momoshtrem/data:/data` | none |
| `momoshtrem-postgres` | `postgres:16-alpine` (16.11) | pulled | none | `/mnt/data/3/momoshtrem/pgdata:/var/lib/postgresql/data` | healthy |
| `jackett` | `lscr.io/linuxserver/jackett:latest` | pulled | **9117** | `/mnt/data/3/jackett:/config` | none |
| `pgadmin` | `dpage/pgadmin4:latest` | pulled | none (reached via Caddy :5050) | anonymous volume `120a4e8b…:/var/lib/pgadmin` | none |

`momoshtrem` has restarted once since the last boot (`RestartCount 1`).

**Environment, names only:**
- `momoshtrem`: `TMDB_API_KEY`, `DATABASE_URL`. The `OPENSUBTITLES_*` variables are set but empty.
- `prettytvcatalog`: `APP_PASSWORD`, `SESSION_SECRET`, `TMDB_API_KEY`, `JACKETT_URL`, `JACKETT_API_KEY`, `MOMOSHTREM_URL`, `SECURE_COOKIES`.
- `postgres`: `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`.
- `pgadmin`: `PGADMIN_DEFAULT_EMAIL`, `PGADMIN_DEFAULT_PASSWORD`, `PGADMIN_LISTEN_PORT`.
- `jackett`: `PUID=1000`, `PGID=1000`, `TZ=UTC`.

**Volumes**
- `apple-tv-tor-stream_caddy_data`: Caddy's internal CA and its certificates. Infuse trusts this CA, and the volume's name follows the project name.
- `apple-tv-tor-stream_caddy_config`: Caddy's autosave, which is regenerated.
- `120a4e8b…`: pgAdmin's anonymous volume.
- `340440d4…` and `4f8d2907…`: dangling anonymous volumes.

**Images on the box**

| Image | Size | State |
|---|---|---|
| `apple-tv-tor-stream-momoshtrem:latest` | 100 MB | in use |
| `apple-tv-tor-stream-prettytvcatalog:latest` | 306 MB | in use |
| `postgres:16-alpine` | 395 MB | in use |
| `dpage/pgadmin4:latest` | 826 MB | in use |
| `lscr.io/linuxserver/jackett:latest` | 275 MB | in use |
| `caddy:2-alpine` | 76 MB | in use |
| `golang:1.25`, `golang:1.24` | 1.26 GB, 1.32 GB | leftovers from building on the box |
| `apple-tv-tor-stream-distribyted:latest` | 381 MB | leftover (pre-momoshtrem) |
| `serfriz/caddy-duckdns:latest` | 136 MB | leftover (ACME experiment) |
| `migrate-to-pg:latest` | 43 MB | leftover (the SQLite → Postgres migration, 2026-02) |

## 7. Ports, end to end

| Port | Published by | LAN | Internet | Notes |
|---|---|---|---|---|
| 22 | sshd | yes | no | keys only |
| 36911 | Caddy | yes | **yes** | WebDAV, `tls internal`, basic auth |
| 3000 | prettytvcatalog | yes | **yes** | old web UI, plain HTTP |
| 9117 | Jackett | yes | no | published to the LAN; D8 says to stop that |
| 5050 | Caddy → pgAdmin | yes | no | Caddy aborts non-192.168.0.0/24 clients |
| 9090 | Caddy → momoshtrem metrics | yes | no | Caddy aborts non-192.168.0.0/24 clients |

## 8. Caddy

The global option `local_certs` puts every site on Caddy's internal CA. The sites are:

| Site | Does |
|---|---|
| `<duckdns-host>:36911` | `tls internal`, `basicauth` for one user (bcrypt hash in the file), `reverse_proxy momoshtrem:36911`, logs to stdout |
| `<duckdns-host>:3000` | `tls internal`, `reverse_proxy prettytvcatalog:3000`. **Dead**: Caddy doesn't publish 3000, and `prettytvcatalog` owns that host port |
| `http://tv.home:9090`, `http://192.168.0.81:9090` | `reverse_proxy momoshtrem:9090`, aborted unless `remote_ip 192.168.0.0/24` |
| `http://tv.home:5050`, `http://192.168.0.81:5050` | `reverse_proxy pgadmin:80`, aborted unless `remote_ip 192.168.0.0/24` |

momoshtrem's own WebDAV auth is off (`webdav_auth.enabled: false`), so **Caddy's basic auth is the only WebDAV auth**. momoshtrem doesn't publish 36911, so every WebDAV client, LAN included, goes through Caddy.

## 9. Data on `/mnt/data`

Everything lives under `/mnt/data/3/` (owned by `agha`, except where noted).

| Path | Size | Owner | Class (PLAN §5) | Notes |
|---|---|---|---|---|
| `momoshtrem/pgdata` | 67 MB | uid 70, mode 700 | Critical | The live library cluster |
| `jackett/` | 396 KB | agha | Critical | `Jackett/ServerConfig.json` holds the API key. Also `Indexers/*.json` (and `.bak`s), `DataProtection/` keys, and `log.txt` (300 KB). `jackett/` (lowercase) is empty |
| `momoshtrem/config/config.yaml` | 635 B | agha | Back up | Holds secrets (§10) |
| `momoshtrem/data/data/subtitles` | 208 KB | root | Back up | 3 files: `movie/10`, `episode/12` and `episode/1170` (`en.srt`). They match the 3 `opensubtitles` rows. The 8 `torrent` subtitle rows point into the torrent cache |
| `momoshtrem/data/data/torrents` | 90 GB | root | Excluded | The torrent cache (`global_cache_size: 92160` MB) |
| `momoshtrem/data/data/cache/vfs_tree.gob` | 1.2 MB | root | Excluded | Rebuilt from the DB |
| `momoshtrem/data/data/momoshtrem.db` (+ `-shm`, 0-byte `-wal`) | 1.2 MB | root | Excluded | Legacy SQLite from before 2026-02-12. It passes `integrity_check` and is archived (§13) |
| `momoshtrem/data/momoshtrem.db` | 0 B | root | Excluded | Empty file |
| `momoshtrem/dat/`, `prettytv/` | empty | agha | none | Leftovers |
| `lost+found/` | | agha | none | |

The path `/data/data/...` is doubled because `config.yaml` uses relative paths (`./data/...`) and the image sets `WORKDIR /data` (PLAN §11).

**config.yaml** (values hidden where secret):

```yaml
server:   { http_port: 4444, webdav_port: 36911, webdav_auth: { enabled: false, username: <set>, password: <set> } }
database: { path: ./data/momoshtrem.db }          # legacy; DATABASE_URL wins
torrent:  { metadata_folder: ./data/torrents, global_cache_size: 92160, add_timeout: 60, read_timeout: 120,
            idle_enabled: true, idle_timeout: 300, start_paused: true }
tmdb:     { api_key: <set> }
opensubtitles: { api_key: <set>, username: <set>, password: <set> }
streaming: { urgent_buffer_bytes: 8388608 }
metrics:  { enabled: true, port: 9090 }
```

## 10. Where secrets live today

| Place | Holds |
|---|---|
| `.env` (checkout) | The keys in §5 |
| `config.yaml` | TMDB key; **OpenSubtitles API key, username and password, which exist nowhere else** (the container's `OPENSUBTITLES_*` env vars are empty); WebDAV user and password (unused while auth is off) |
| `Caddyfile` (checkout) | WebDAV basic-auth user and bcrypt hash |
| `caddy_data` volume | Root and intermediate CA private keys, plus the site key |
| Jackett config | The API key, which must match `JACKETT_API_KEY`. Also indexer settings and DataProtection keys |
| Postgres | Role password hash (`pg_dumpall --globals-only`) |

## 11. Boot and scheduled jobs

`/etc/systemd/system/streaming-stack.service` is enabled:

```ini
[Unit]
Description=AppleTV Streaming Stack
Requires=docker.service
After=docker.service network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/streaming-stack
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

**It doesn't work.**
- `/opt/streaming-stack` doesn't exist; the checkout moved to `/opt/streaming-stack-3/...`.
- At the last boot (2026-10-11 16:14 UTC) it failed with `status=200/CHDIR`, and it has stayed `failed` since.
- The containers came back through `restart: unless-stopped`.
- So a `docker compose down`, followed by a reboot, would leave the stack stopped.

**Scheduled jobs**
- There are no user or root crontabs (`/var/spool/cron/crontabs` is empty).
- `/etc/cron.d` has only the stock `e2scrub_all` and `sysstat`.
- The timers are all stock Ubuntu: apt, logrotate, man-db, fstrim, e2scrub, sysstat, fwupd, motd.
- **No backup and no DuckDNS job exists.**

## 12. Other things only on the box

- **Database.** PostgreSQL 16.11, database `momoshtrem`, 12 MB. `schema_migrations` is at version 1 (2026-02-12).

  Row counts on 2026-10-14:

  | Table | Rows |
  |---|---|
  | `episodes` | 2167 |
  | `seasons` | 170 |
  | `shows` | 56 |
  | `movies` | 38 |
  | `torrent_assignments` | 1713 |
  | `subtitles` | 11 |
  | `sync_metadata` | 1 |
  | `schema_migrations` | 1 |

- **`agha`'s home.**
  - `~/.claude/` (Claude Code sessions run on the box);
  - `~/.vscode-server/` (VS Code remote);
  - `~/.bash_history`, which contains the DuckDNS token in plain text from the TXT experiments.

  None of it is needed to run the stack.
- **The original install notes** (kept privately, not in the repo) describe a plan that drifted. The VM was meant to be at .150 with a 200 GB data disk under `/opt/streaming-stack`; it ended up at .81 with 120 GB under `/opt/streaming-stack-3`.

## 13. The safety-net copy (#33)

On 2026-10-14 these were copied to the Mac, outside the repo, in a folder only the user can read. They were streamed over SSH (`docker exec … pg_dump|tar`), so nothing was written on the VM. The folder holds secrets. Delete it once #35 has verified the B2 backups.

| File | What |
|---|---|
| `library.dump` | `pg_dump -Fc` of `momoshtrem` |
| `globals.sql` | `pg_dumpall --globals-only` (roles) |
| `row-counts.sql`, `row-counts.txt` | The count query and the live counts at dump time |
| `jackett-config.tar` | Jackett `/config` |
| `momoshtrem-config.yaml` | `config.yaml` |
| `subtitles.tar` | The downloaded subtitles |
| `legacy-sqlite.tar` | The legacy SQLite file with its `-shm`/`-wal`, and the 0-byte stray |
| `checkout-files.tar` | `.env`, `Caddyfile`, `compose.override.yml`, `compose.yml` |
| `git-stash-compose.patch` | `git stash show -p stash@{0}` |
| `caddy_data.tar` | The `caddy_data` volume |
| `MANIFEST.txt` | Sources, the commit, and sha256 checksums |

**Restore check.** `library.dump` was restored into a throwaway `postgres:16-alpine` on the Mac (`pg_restore --no-owner --exit-on-error`, exit 0). The row counts matched the live database exactly.

**Not copied** (the Proxmox snapshot covers them):
- the torrent cache;
- `vfs_tree.gob`;
- `caddy_config`;
- pgAdmin's volume;
- the stray `./data/pgdata`;
- the images.
