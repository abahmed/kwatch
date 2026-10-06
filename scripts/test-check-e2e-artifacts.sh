#!/bin/sh
# Checks scripts/check-e2e-artifacts.sh: prose is not flagged, credentials
# are, and --redact removes only the unsafe line.
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
check="$root_dir/scripts/check-e2e-artifacts.sh"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

printf 'basic usage of the API\n' >"$tmp_dir/prose.log"
sh "$check" "$tmp_dir" 2>/dev/null || {
	echo "FAIL: prose containing 'basic' must pass" >&2
	exit 1
}

printf 'keep me\nAuthorization: Basic dXNlcjpwYXNz\nkeep too\n' \
	>"$tmp_dir/unsafe.log"
if sh "$check" "$tmp_dir" 2>/dev/null; then
	echo "FAIL: a Basic credential must be flagged" >&2
	exit 1
fi
sh "$check" --redact "$tmp_dir" || {
	echo "FAIL: redaction must leave a clean directory" >&2
	exit 1
}
grep -q 'keep me' "$tmp_dir/unsafe.log" &&
	grep -q 'keep too' "$tmp_dir/unsafe.log" || {
	echo "FAIL: redaction must keep the safe lines" >&2
	exit 1
}
echo "e2e artifact check: PASS"
