#!/usr/bin/env bash
# The VM half of ops/deploy/deploy.sh, which copies this file to a temp folder
# on the VM, next to the target commit's compose.yml and Caddyfile, and runs:
#
#   remote.sh status                    print the deploy state (read-only)
#   remote.sh check  <sha>              preflight only, changes nothing
#   remote.sh deploy <sha> <timeout>    deploy <sha>
#
# deploy exits 1 when it stopped before changing anything, and 2 when <sha> is
# installed but not healthy.
#
# Runbook: docs/ops/DEPLOY.md. Contract: docs/ops/PLAN.md §8.
# Never print .env: it holds every secret (PLAN D6).
set -euo pipefail

APP=/opt/momoshtrem
STAGE=$(cd "$(dirname "$0")" && pwd)
STATE=$APP/deploy-state
LOG=$APP/deploy.log
DUMPS=$APP/dumps
KEEP_DUMPS=10
# Shared with the backup job (#35), so the two never run at once
LOCK=/run/lock/momoshtrem.lock
OURS="ghcr.io/shapedtime/momoshtrem ghcr.io/shapedtime/momoshtrem-web"

die() { echo "error: $*" >&2; exit 1; }
step() { echo "==> $*"; }

# The deployed stack, and the target commit's compose file run against the
# same folder and .env
live() { docker compose --progress quiet "$@"; }
staged() {
	MOMOSHTREM_TAG=$sha docker compose --progress quiet -f "$STAGE/compose.yml" \
		--project-directory "$APP" --env-file "$APP/.env" "$@"
}

state_get() { [ -f "$STATE" ] && sed -n "s/^$1=//p" "$STATE" | tail -n 1 || true; }
# The running commit. Before the first scripted deploy there's no state file,
# and the tag a hand deploy left in .env is it.
running() { if [ -f "$STATE" ]; then state_get current; else env_tag; fi; }
env_tag() { [ -f .env ] && sed -n 's/^MOMOSHTREM_TAG=//p' .env | tail -n 1 | tr -d "\"'" || true; }
file_sha() { [ -f "$1" ] && sha256sum "$1" | cut -d' ' -f1 || true; }

write_state() { # current previous last_failed
	local tmp
	tmp=$(mktemp "$APP/.deploy-state.XXXXXX")
	printf 'current=%s\nprevious=%s\nlast_failed=%s\nupdated=%s\n' \
		"$1" "$2" "$3" "$(date -u +%FT%TZ)" >"$tmp"
	mv "$tmp" "$STATE"
}

log_result() { # result
	printf '%s %s %s -> %s\n' "$(date -u +%FT%TZ)" "$1" "${from:-none}" "$sha" >>"$LOG"
}

# Services that aren't running and healthy. Every service has a healthcheck
# (#37), and a crash-looping one shows no health between restarts, so running
# alone doesn't count.
not_ready() {
	local states s line
	states=$(live ps -a --format '{{.Service}} {{.State}}/{{.Health}}')
	for s in $(live config --services); do
		line=$(sed -n "s/^$s //p" <<<"$states" | head -n 1)
		case $line in
		running/healthy) ;;
		*) printf '%s(%s) ' "$s" "${line:-missing}" ;;
		esac
	done
}

status() {
	echo "tag=$(env_tag)"
	echo "env=$([ -f .env ] && echo yes || echo no)"
	echo "current=$(running)"
	for k in previous last_failed updated; do
		echo "$k=$(state_get "$k")"
	done
	echo "compose_sha256=$(file_sha compose.yml)"
	echo "caddyfile_sha256=$(file_sha Caddyfile)"
	if [ -f compose.yml ] && [ -f .env ]; then
		live ps -a --format 'service={{.Service}} {{.State}} {{.Health}}' 2>/dev/null || true
	fi
}

preflight() {
	[ -f "$APP/.env" ] || die "$APP/.env is missing: write it first (docs/ops/DEPLOY.md §3)"
	docker info >/dev/null 2>&1 || die "can't reach Docker as $(id -un)"
	staged config --quiet ||
		die "compose.yml from ${sha:0:7} doesn't accept this .env: add the variables named above"
}

dump() {
	if [ -z "$(live ps --status running -q postgres 2>/dev/null || true)" ]; then
		echo "postgres isn't running: no pre-deploy dump (first deploy)"
		return 0
	fi
	local f was=${from:-none}
	f="$DUMPS/$(date -u +%Y%m%dT%H%M%SZ)-${was:0:7}.dump"
	(umask 077 && mkdir -p "$DUMPS")
	if ! (umask 077 && live exec -T postgres pg_dump -U momoshtrem -Fc momoshtrem >"$f") || [ ! -s "$f" ]; then
		rm -f "$f"
		die "the pre-deploy dump failed: nothing has changed"
	fi
	echo "$f ($(du -h "$f" | cut -f1))"
	# Newest first; keep KEEP_DUMPS
	find "$DUMPS" -maxdepth 1 -name '*.dump' -printf '%T@ %p\n' | sort -rn |
		tail -n +$((KEEP_DUMPS + 1)) | cut -d' ' -f2- | xargs -r rm -f
}

# Remove old versions of the images the stack uses. Keep every image a
# container uses, plus ours at the previous commit for a quick rollback.
prune() {
	local in_use repos ref
	in_use=$(docker ps -a --format '{{.Image}}')
	# shellcheck disable=SC2086 # one image per word
	repos=$(printf '%s\n' $in_use $OURS | sed 's/:[^:/]*$//' | sort -u)
	for ref in $(docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' || true); do
		grep -qxF "${ref%:*}" <<<"$repos" || continue
		grep -qxF "$ref" <<<"$in_use" && continue
		case " $OURS " in *" ${ref%:*} "*) [ "${ref##*:}" = "$previous" ] && continue ;; esac
		docker image rm "$ref" >/dev/null && echo "removed $ref"
	done
	docker image prune -f >/dev/null
}

# After the install step, any failure lands here: record it, show why, and
# leave the stack as it is. There's no automatic rollback (PLAN §8).
on_exit() {
	local rc=$?
	if [ "$rc" -ne 0 ] && [ "${phase:-}" = installed ]; then
		set +e
		echo
		echo "==> FAILED: ${sha:0:7} is installed but not healthy. Nothing was rolled back."
		live ps -a --format 'table {{.Service}}\t{{.Status}}\t{{.Image}}'
		for s in $(not_ready | sed 's/([^)]*)//g'); do
			echo "--- $s: last 30 log lines"
			live logs --no-color --tail 30 "$s" 2>&1
		done
		write_state "$from" "$previous" "$sha"
		log_result failed
		exit 2
	fi
	exit "$rc"
}

deploy() {
	[ -e "$LOCK" ] || (umask 0111 && : >"$LOCK")
	exec 9<"$LOCK"
	flock -n 9 || die "another deploy or backup is running ($LOCK)"

	preflight
	from=$(running)
	previous=$(state_get previous)
	last_failed=$(state_get last_failed)

	step "pre-deploy dump"
	dump

	step "pull"
	staged pull || die "pull failed: nothing has changed"

	step "install compose.yml, Caddyfile and MOMOSHTREM_TAG"
	trap on_exit EXIT
	phase=installed
	caddy_changed=yes
	cmp -s "$STAGE/Caddyfile" Caddyfile && caddy_changed=no
	# cp writes into the existing file, so caddy's single-file bind mount
	# sees the new content
	cp "$STAGE/compose.yml" compose.yml
	cp "$STAGE/Caddyfile" Caddyfile
	if grep -q '^MOMOSHTREM_TAG=' .env; then
		sed -i "s/^MOMOSHTREM_TAG=.*/MOMOSHTREM_TAG=$sha/" .env
	else
		printf 'MOMOSHTREM_TAG=%s\n' "$sha" >>.env
	fi

	step "up"
	live up -d --remove-orphans

	step "wait until every service is healthy (up to ${timeout}s)"
	local deadline=$((SECONDS + timeout)) waiting last=""
	while :; do
		waiting=$(not_ready)
		[ -n "$waiting" ] || break
		[ "$SECONDS" -lt "$deadline" ] || die "not healthy after ${timeout}s: $waiting"
		[ "$waiting" = "$last" ] || echo "waiting for: $waiting"
		last=$waiting
		sleep 5
	done

	if [ "$caddy_changed" = yes ]; then
		# Caddy doesn't watch its file. A bad config is refused, and the old
		# one keeps running.
		step "reload Caddy"
		live exec -T caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
	fi

	step "check the version /api/status reports"
	local version
	# shellcheck disable=SC2016 # expanded inside the container
	version=$(live exec -T momoshtrem sh -c \
		'wget -qO- --header "X-Momoshtrem-Token: $BACKEND_API_TOKEN" http://127.0.0.1:4444/api/status' |
		sed -n 's/.*"version" *: *"\([^"]*\)".*/\1/p' || true)
	[ "$version" = "$sha" ] || die "/api/status reports '${version}', expected $sha"
	echo "$version"

	phase=finished
	[ "$from" = "$sha" ] || previous=$from
	[ "$last_failed" = "$sha" ] && last_failed=""
	write_state "$sha" "$previous" "$last_failed"
	log_result ok

	step "prune old images"
	prune || echo "warning: pruning failed; the deploy itself succeeded"
}

mode=${1:-}
sha=${2:-}
timeout=${3:-300}
case $mode in
status | check | deploy) ;;
*) die "usage: remote.sh status | check <sha> | deploy <sha> <timeout>" ;;
esac
[ "$mode" = status ] || [[ $sha =~ ^[0-9a-f]{40}$ ]] || die "not a full commit SHA: $sha"
[[ $timeout =~ ^[0-9]+$ ]] || die "not a number of seconds: $timeout"
cd "$APP" 2>/dev/null || die "$APP doesn't exist: run the host playbook first (docs/ops/HOST.md)"

case $mode in
status) status ;;
check) preflight && echo "preflight ok: compose.yml from ${sha:0:7} accepts this .env" ;;
deploy) deploy ;;
esac
