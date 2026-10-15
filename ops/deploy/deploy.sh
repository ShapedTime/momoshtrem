#!/usr/bin/env bash
# Deploy a green main commit to a momoshtrem VM over SSH. Run it on the Mac,
# from anywhere in the repo. GitHub never reaches the VM (ops PLAN D3).
#
#   ops/deploy/deploy.sh [-n] [-y] [--timeout SEC] <ssh-host> [<commit>]
#
#   <ssh-host>     an SSH destination, for example the alias momoshtrem-v2
#   <commit>       a main commit with images on GHCR (default: the newest one)
#   -n, --dry-run  print the plan and run the preflight on the VM; change nothing
#   -y, --yes      don't ask before deploying
#   --timeout SEC  how long to wait for every service to be healthy (default 300)
#
# Rolling back is a deploy of the previous commit. Runbook: docs/ops/DEPLOY.md.
set -euo pipefail

usage() {
	sed -n '5,11s/^# \{0,1\}//p' "$0" >&2
	exit "${1:-1}"
}
die() {
	echo "error: $*" >&2
	exit 1
}

dry=no yes=no timeout=300
while [ $# -gt 0 ]; do
	case $1 in
	-n | --dry-run) dry=yes ;;
	-y | --yes) yes=yes ;;
	--timeout)
		[ $# -ge 2 ] || usage
		timeout=$2
		shift
		;;
	-h | --help) usage 0 ;;
	-*) usage ;;
	*) break ;;
	esac
	shift
done
[ $# -ge 1 ] && [ $# -le 2 ] || usage
host=$1 want=${2:-}
case $timeout in '' | *[!0-9]*) die "--timeout takes a number of seconds" ;; esac

here=$(cd "$(dirname "$0")" && pwd)
cd "$(git -C "$here" rev-parse --show-toplevel)"
git fetch -q origin main

# Both images exist for <sha>. GHCR answers anonymously: the images are public
# (ops PLAN D7).
token_momoshtrem="" token_web=""
has_images() {
	local img token code
	for img in momoshtrem momoshtrem-web; do
		if [ "$img" = momoshtrem ]; then token=$token_momoshtrem; else token=$token_web; fi
		if [ -z "$token" ]; then
			token=$(curl -fsS "https://ghcr.io/token?scope=repository:shapedtime/$img:pull" |
				sed -n 's/.*"token" *: *"\([^"]*\)".*/\1/p') || die "can't reach ghcr.io"
			if [ "$img" = momoshtrem ]; then token_momoshtrem=$token; else token_web=$token; fi
		fi
		code=$(curl -sS -o /dev/null -w '%{http_code}' -I \
			-H "Authorization: Bearer $token" \
			-H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json' \
			"https://ghcr.io/v2/shapedtime/$img/manifests/$1") || die "can't reach ghcr.io"
		case $code in
		200) ;;
		404) return 1 ;;
		*) die "ghcr.io answered HTTP $code for $img:$1" ;;
		esac
	done
}

title() { git log -1 --format='%h %s' "$1" 2>/dev/null || echo "${1:0:7} (not in this clone)"; }

# The target: the given commit, or the newest main commit with images
if [ -n "$want" ]; then
	sha=$(git rev-parse --verify --quiet "$want^{commit}") || die "unknown commit: $want"
	git merge-base --is-ancestor "$sha" origin/main || die "$want isn't on origin/main"
	has_images "$sha" || die "$want has no images on GHCR: its publish jobs didn't run or failed (docs/ops/CI.md)"
else
	sha=""
	for c in $(git rev-list --first-parent -n 30 origin/main); do
		if has_images "$c"; then
			sha=$c
			break
		fi
		echo "skipping $(title "$c"): no images yet (CI running or failed)"
	done
	[ -n "$sha" ] || die "none of the last 30 main commits has images"
fi

# remote.sh, the target's compose.yml and its Caddyfile travel as one tar
# over a single SSH connection, into a temp folder that's removed afterwards
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
cp "$here/remote.sh" "$stage/"
git show "$sha:compose.yml" >"$stage/compose.yml"
git show "$sha:Caddyfile" >"$stage/Caddyfile"
remote() {
	# The arguments are a mode, a SHA and a number, so expanding them here is safe
	# shellcheck disable=SC2029
	COPYFILE_DISABLE=1 tar -C "$stage" -cf - remote.sh compose.yml Caddyfile |
		ssh "$host" "d=\$(mktemp -d) && trap 'rm -rf \"\$d\"' EXIT &&
			tar --warning=no-unknown-keyword -xf - -C \"\$d\" && bash \"\$d/remote.sh\" $*"
}

state=$(remote status) || die "can't read the deploy state on $host"
get() { sed -n "s/^$1=//p" <<<"$state" | head -n 1; }
current=$(get current)
last_failed=$(get last_failed)
[ "$(get env)" = yes ] || die "$host has no /opt/momoshtrem/.env: write it first (docs/ops/DEPLOY.md §3)"

# The plan
known=no
[ -n "$current" ] && git cat-file -e "$current^{commit}" 2>/dev/null && known=yes
echo "Host:     $host"
if [ -n "$current" ]; then echo "Running:  $(title "$current")"; else echo "Running:  nothing yet"; fi
installed=$(get tag)
if [ -n "$installed" ] && [ "$installed" != "$current" ]; then
	echo "          but $(title "$installed") is installed: its deploy didn't finish"
fi
echo "Target:   $(title "$sha")"
services=$(sed -n 's/^service=//p' <<<"$state" | sed 's/ *$//' | tr '\n' ',' | sed 's/,$//; s/,/, /g')
[ -z "$services" ] || echo "Services: $services"

if [ "$current" = "$sha" ]; then
	echo
	echo "Already running ${sha:0:7}. Nothing to do."
	exit 0
fi

direction=upgrade
if [ "$known" = yes ]; then
	echo
	if git merge-base --is-ancestor "$current" "$sha"; then
		echo "Upgrade, adding:"
		git log --first-parent --format='  + %h %s' "$current..$sha"
	elif git merge-base --is-ancestor "$sha" "$current"; then
		direction=rollback
		echo "Rollback, removing:"
		git log --first-parent --format='  - %h %s' "$sha..$current"
	fi

	migrations=$(git diff --name-status "$current" "$sha" -- momoshtrem/internal/library/migrations/)
	if [ -z "$migrations" ]; then
		echo "Migrations: none crossed"
	else
		echo "Migrations crossed:"
		git diff --name-status "$current" "$sha" -- momoshtrem/internal/library/migrations/ | sed 's/^/  /'
		if [ "$direction" = rollback ]; then
			echo "  WARNING: migrations only go forward. To roll the data back too, restore"
			echo "  the dump taken before the newer deploy (docs/ops/DEPLOY.md §2)."
		fi
	fi

	for f in compose.yml Caddyfile; do
		if git diff --quiet "$current" "$sha" -- "$f"; then echo "$f: unchanged"; else echo "$f: changed"; fi
		case $f in compose.yml) vm=$(get compose_sha256) ;; *) vm=$(get caddyfile_sha256) ;; esac
		if [ -z "$vm" ]; then
			echo "  note: the VM has no $f yet."
		elif [ "$(git show "$current:$f" | shasum -a 256 | cut -d' ' -f1)" != "$vm" ]; then
			echo "  note: the VM's $f differs from ${current:0:7}'s (edited by hand?). It will be replaced."
		fi
	done

	if ! git diff --quiet "$current" "$sha" -- .env.example; then
		echo
		echo ".env.example differs between the two. The VM's .env is managed by hand (PLAN D6):"
		echo "check it before deploying."
		git --no-pager diff --color=never "$current" "$sha" -- .env.example | sed 's/^/  /'
	fi
elif [ -n "$current" ]; then
	echo "The running commit isn't in this clone: no changelog or migration check."
fi

if [ -n "$last_failed" ] && [ "$last_failed" = "$sha" ]; then
	echo
	echo "Note: ${sha:0:7} failed on its last deploy to $host ($(get updated))."
fi
echo

if [ "$dry" = yes ]; then
	remote check "$sha"
	echo "Dry run: nothing changed."
	exit 0
fi

if [ "$yes" != yes ]; then
	read -r -p "Deploy ${sha:0:7} to $host? [y/N] " answer
	case $answer in y | Y | yes) ;; *) die "cancelled: nothing changed" ;; esac
fi

# remote.sh exits 1 when it stopped before changing anything, and 2 when the
# target is installed but not healthy
rc=0
remote deploy "$sha" "$timeout" || rc=$?
echo
case $rc in
0) echo "Deployed ${sha:0:7} to $host." ;;
1) die "the deploy stopped before changing anything on $host" ;;
2)
	echo "${sha:0:7} is installed on $host but isn't healthy. Nothing was rolled back."
	[ -z "$current" ] || echo "To roll back: ops/deploy/deploy.sh $host ${current:0:7}"
	exit 1
	;;
*) die "the deploy ended with code $rc (did SSH drop?). Run with -n to see the VM's state." ;;
esac
