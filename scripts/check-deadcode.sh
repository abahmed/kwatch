#!/bin/sh

# Fail on functions no binary or test reaches. Generated deepcopy code is
# ignored; every other exception is a line in scripts/deadcode-allowlist.txt
# ("path: Name", no line numbers) with a comment saying why.
set -eu

version=${DEADCODE_VERSION:-v0.52.0}
allow=scripts/deadcode-allowlist.txt

out=$(mktemp)
trap 'rm -f "$out"' EXIT
go run "golang.org/x/tools/cmd/deadcode@$version" -test ./... >"$out"

found=$(grep -v '/zz_generated\.' "$out" |
	sed -E 's/^([^:]+):[0-9]+:[0-9]+: unreachable func: /\1: /' || true)

new=$(printf '%s\n' "$found" |
	grep -vxF -f "$allow" || true)

if [ -n "$(printf '%s' "$new" | tr -d '[:space:]')" ]; then
	echo "unreachable functions (delete them, or justify in $allow):" >&2
	printf '%s\n' "$new" >&2
	exit 1
fi

# An allowlist line nothing matches any more is stale: delete it, so the
# list cannot rot into a blanket exemption.
entries=$(mktemp)
hits=$(mktemp)
trap 'rm -f "$out" "$entries" "$hits"' EXIT
grep -v '^[[:space:]]*#' "$allow" |
	grep -v '^[[:space:]]*$' >"$entries" || true
printf '%s\n' "$found" >"$hits"
stale=$(grep -vxF -f "$hits" "$entries" || true)
if [ -n "$stale" ]; then
	echo "stale entries in $allow (delete them):" >&2
	printf '%s\n' "$stale" >&2
	exit 1
fi
echo "deadcode-check: no unreachable functions"
