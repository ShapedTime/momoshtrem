# The host playbook

> Runbook · 2026-10-15 · #34 · Epic #32 · Plan: [`PLAN.md`](PLAN.md)
> `ops/ansible/` configures the momoshtrem VM after it's created by hand from [`VM.md`](VM.md). Run it from the Mac. It's safe to run again at any time.

The playbook owns the OS layer (PLAN §4):
- packages and unattended upgrades;
- Docker and its log caps;
- the journald cap;
- the guest agent;
- ufw;
- the data folders;
- the boot unit.

It doesn't own:
- `.env` and `config.yaml`, which stay hand-managed (D6);
- the data (#42);
- the backups (#35);
- the deploy agent (#38). #35 and #38 add their timers here (§7).

## 1. What it does

`site.yml` is one play. Each section is a task file with a tag of the same name, so `--tags docker` runs just that part.

| Tag | Changes | Restarts |
|---|---|---|
| `handoff` | Nothing. It checks the VM.md §5 state and stops the run if any of it is off: Ubuntu 24.04, the hostname, `ubuntu-lv` filling the VG, `/mnt/data` on ext4 by UUID, no Docker snap, and `agha` with uid 1000. Always runs | |
| `base` | Base packages. `20auto-upgrades` turns on daily security updates. Ubuntu's defaults stay: security only, and no automatic reboot | |
| `guest_agent` | Installs and starts `qemu-guest-agent` | |
| `docker` | Docker's apt repo, the engine and the compose plugin. `daemon.json` sets the `json-file` driver, capped at 3 × 10 MB per container. `agha` joins the `docker` group | dockerd, when `daemon.json` changes (§3) |
| `journald` | `journald.conf.d/10-size.conf`: `SystemMaxUse=500M` | journald |
| `firewall` | ufw denies incoming and routed traffic, allows outgoing, and allows 22/tcp from `lan_cidr` | |
| `folders` | The data and app folders (§4) | |
| `boot` | `momoshtrem-stack.service` (§5) | |

**Values** live in `ops/ansible/group_vars/all.yml`, and none are secret. The inventory points at the Mac's SSH alias `momoshtrem-v2`, so no IP is in the repo.

**The disks.** The playbook never partitions or formats a disk. The installer sets them up (VM.md §4), and `handoff` checks the result.

## 2. Running it

**Once, on the Mac.** Install Ansible with `pipx install --include-deps ansible`. It includes `community.general`. With a bare `ansible-core`, also run `ansible-galaxy collection install -r requirements.yml`.

**Every run.** Run from `ops/ansible/`, so `ansible.cfg` is picked up:

```bash
cd ops/ansible
ansible-playbook site.yml --check --diff   # dry run: read the diff first
ansible-playbook site.yml                  # real run
ansible-playbook site.yml --tags docker    # one section
```

- **The sudo password.** Ansible asks for it (`BECOME password:`). It's `agha`'s password from the password manager.
- **The routine.** Run in check mode before every real run.
- **Snapshots.** Take a Proxmox snapshot before the first real run on a VM, and before any change to Docker, ufw or the boot unit.
- **Done.** A second real run reports `changed=0`.

**Check mode on a bare VM.** A dry run can't install anything, so it skips the tasks that need what the same run would install:
- the Docker service, group and `daemon.json`;
- the guest-agent service;
- enabling the boot unit;
- the journald drop-in.

They show as skipped, not failed. The first real run does them.

**Agent-run sessions.** An agent can't answer the prompt. For that session only, the user puts the password in a gitignored file, then deletes it afterwards (PLAN §7):

```bash
# On the Mac, in ops/ansible. The password doesn't reach the shell history.
( umask 077; read -rs 'p?sudo password: '; echo; printf '%s\n' "$p" > sudo-pass.txt )

# The agent runs (the env var turns off the prompt):
ANSIBLE_BECOME_ASK_PASS=false ansible-playbook site.yml --check --diff --become-password-file sudo-pass.txt

rm sudo-pass.txt   # when the session ends
```

## 3. What restarts what

- **`daemon.json` changes.** Restarting dockerd bounces every container for a few seconds, and `restart: unless-stopped` brings them back.
  - On a VM's first run, dockerd starts with the file already in place, so nothing restarts.
  - Log options only reach containers created after the change. To apply them to the running stack, run `docker compose up -d --force-recreate` in `/opt/momoshtrem`.
- **Docker upgrades** never happen unattended: Docker's repo isn't in the unattended-upgrades origins. Upgrade it by hand with `sudo apt upgrade`, at a quiet time, because it restarts dockerd.
- **Kernel and security updates** install daily but don't reboot. Reboot by hand when `/var/run/reboot-required` exists. The boot unit brings the stack back (§5).

## 4. Folders

Created with these owners and modes, never recursed into, never deleted:

| Path | Owner | Mode | Holds |
|---|---|---|---|
| `/mnt/data/momoshtrem/config` | agha | 0755 | `config.yaml` (hand-managed) |
| `/mnt/data/momoshtrem/data` | root | 0755 | The torrent cache, `subtitles/` and `cache/`. The container runs as root, and the paths inside are `data/...` (PLAN §11) |
| `/mnt/data/momoshtrem/pgdata` | 70:70 | 0700 | The Postgres cluster (uid 70 is `postgres` in `postgres:16-alpine`) |
| `/mnt/data/jackett` | agha | 0755 | Jackett `/config` (`PUID=1000`) |
| `/opt/momoshtrem` | agha | 0755 | The deployed compose file and `.env` (#38) |

These replace the old VM's `/mnt/data/3/...` paths (AS-IS §9). `.env` points compose at them (#37):

```env
MOMOSHTREM_CONFIG=/mnt/data/momoshtrem/config
MOMOSHTREM_DATA=/mnt/data/momoshtrem/data
MOMOSHTREM_PGDATA=/mnt/data/momoshtrem/pgdata
JACKETT_DATA=/mnt/data/jackett
```

## 5. The boot unit

`momoshtrem-stack.service` runs `docker compose up -d` in `/opt/momoshtrem` at boot. The old VM's unit pointed at a missing folder and failed (AS-IS §11). This one is fixed in three ways:

- **It waits for a deploy.** `ConditionPathExists=/opt/momoshtrem/compose.yml` means that until #38 deploys, the unit is skipped, not failed.
- **It has no `ExecStop`.** Shutdown stops the containers anyway. A `docker compose down` would remove them, and the stack would then depend on this unit to come back.
- **The contract with #38.** The compose file and `.env` in `app_dir` must describe the deployed commit on their own, image tag included. Nothing is passed on the command line, so a plain `up -d` at boot brings back exactly what was deployed. `app_dir` is a variable, and #38 may move it.

## 6. Firewall and lockout recovery

- **What ufw covers.** It guards host services only. Docker-published ports bypass it, because Docker writes its own iptables rules, so compose decides what the containers expose (#37).
- **The lockout guard.** Before turning ufw on, the playbook checks that its own SSH session comes from `lan_cidr`. If it doesn't, the run stops, because the rule would lock the next login out.
  - Run it from the LAN.
  - Or, if the LAN changed, change `lan_cidr` first.

**If SSH is locked out anyway:**
1. Open the VM's console in Proxmox and log in as `agha`.
2. Run `sudo ufw disable`.
3. Fix `lan_cidr` and run the playbook again.

## 7. Adding to it

#35 (backups) and #38 (the deploy agent) install their timers here:
1. Add a task file under `tasks/`.
2. Import it in `site.yml` with its own tag.
3. Put its templates in `templates/`.

Keep these rules:
- **Values.** Non-secret values go in `group_vars/all.yml`. Secrets stay on the VM (D6).
- **Check mode.** A task that needs something the same run creates gets the same guard as the others (`when: not (ansible_check_mode and …)`), so a dry run on a bare VM still passes.
- **Lint.** `ansible-lint site.yml`, run from `ops/ansible/`, must pass at the `production` profile.
- **The final check.** A second real run reports `changed=0`.
