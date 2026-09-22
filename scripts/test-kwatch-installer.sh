#!/bin/sh

set -eu

: "${INSTALLER_URL:=https://kwatch.dev/kwatch.sh}"
: "${ARTIFACTS:=$(mktemp -d)}"

command -v bash >/dev/null 2>&1 || exit 2
command -v curl >/dev/null 2>&1 || exit 2
command -v kubectl >/dev/null 2>&1 || exit 2
mkdir -p "$ARTIFACTS"

installer="$ARTIFACTS/kwatch.sh"
curl -fsSL --retry 3 --retry-all-errors "$INSTALLER_URL" \
	-o "$installer"

# The downloaded installer is treated as an artifact under test. It is never
# sourced into this process, and no issue or workflow input is evaluated as
# shell code.
bash -n "$installer"
bash "$installer" --help >"$ARTIFACTS/help.txt"
bash "$installer" --version >"$ARTIFACTS/version.txt"
grep -Fq "Interactive kubectl manager" "$ARTIFACTS/help.txt"
grep -Eq 'catalog version [0-9]+' "$ARTIFACTS/version.txt"

kubectl version --client >/dev/null
printf '%s\n' "installer smoke checks passed"
