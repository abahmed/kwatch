#!/bin/sh

set -eu
# pipefail is not POSIX; enable it where the shell supports it.
# shellcheck disable=SC3040
(set -o pipefail) 2>/dev/null && set -o pipefail

root_dir=${KWATCH_REPO_ROOT:-$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)}
cd "$root_dir"

: "${REPORTED_REF:?REPORTED_REF is required for compare mode}"
: "${ARTIFACTS:=$(mktemp -d)}"
: "${KIND_CLUSTER_NAME:=kwatch-e2e}"
: "${KEEP_CLUSTER:=false}"

# REPORTED_REF comes from a workflow input. Accept only a release tag, a
# branch-like name, or a commit SHA, so it can never become a git option or
# a refspec with a destination.
ref_pattern='[0-9a-f]{7,40}|v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?'
ref_pattern="$ref_pattern|[A-Za-z0-9][A-Za-z0-9._/-]{0,99}"
case "$REPORTED_REF" in
*[!A-Za-z0-9._/-]*|'')
	echo "REPORTED_REF contains characters outside [A-Za-z0-9._/-]" >&2
	exit 2
	;;
esac
if ! printf '%s\n' "$REPORTED_REF" | grep -Eqx "$ref_pattern" ||
	printf '%s\n' "$REPORTED_REF" | grep -Eq '\.\.|//|/$|\.lock$|@\{'; then
	echo "REPORTED_REF is not a tag, branch or commit: $REPORTED_REF" >&2
	exit 2
fi

reported_root=""
reported_status=2
main_status=2

# json_escape prints $1 as a JSON string body: backslash, quote and control
# characters are escaped. jq is used when present.
json_escape() {
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$1" | jq -Rrs '@json | .[1:-1]'
		return
	fi
	printf '%s' "$1" | awk '
		BEGIN { ORS = "" }
		{
			if (NR > 1) printf "\\n"
			gsub(/\\/, "\\\\")
			gsub(/"/, "\\\"")
			gsub(/\t/, "\\t")
			gsub(/\r/, "\\r")
			print
		}'
}

cleanup() {
	status=$?
	if [ -n "$reported_root" ]; then
		git worktree remove --force "$reported_root" >/dev/null 2>&1 || true
	fi
	exit "$status"
}

trap cleanup EXIT INT TERM

mkdir -p "$ARTIFACTS/reported" "$ARTIFACTS/main"
reported_sha=$(git rev-parse --verify --quiet \
	"$REPORTED_REF^{commit}" 2>/dev/null || true)
if [ -z "$reported_sha" ]; then
	git fetch --depth=1 -- origin "$REPORTED_REF"
	reported_sha=$(git rev-parse 'FETCH_HEAD^{commit}')
fi

reported_root=$(mktemp -d "${TMPDIR:-/tmp}/kwatch-reported.XXXXXX")
rmdir "$reported_root"
git worktree add --detach "$reported_root" "$reported_sha" >/dev/null

for required_file in Dockerfile deploy/crd.yaml deploy/deploy.yaml; do
	if [ ! -f "$reported_root/$required_file" ]; then
		cat >"$ARTIFACTS/compare.json" <<EOF
{
  "reportedRef": "$(json_escape "$REPORTED_REF")",
  "reportedSHA": "$reported_sha",
  "classification": "unsupported_version",
  "missingFile": "$required_file"
}
EOF
		exit 1
	fi
done

set +e
KWATCH_HARNESS_ROOT="$root_dir" \
KWATCH_CANDIDATE_ROOT="$reported_root" \
	KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME}-reported" \
	ARTIFACTS="$ARTIFACTS/reported" \
	KEEP_CLUSTER="$KEEP_CLUSTER" \
	SOURCE_REF="$REPORTED_REF" \
	KWATCH_SOURCE_SHA="$reported_sha" \
	./scripts/test-kind-scenarios.sh
reported_status=$?

KWATCH_REPO_ROOT="$root_dir" \
	KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME}-main" \
	ARTIFACTS="$ARTIFACTS/main" \
	KEEP_CLUSTER="$KEEP_CLUSTER" \
	SOURCE_REF="main" \
	KWATCH_SOURCE_SHA="$(git rev-parse HEAD)" \
	./scripts/test-kind-scenarios.sh
main_status=$?
set -e

classification="still_failing"
if [ "$reported_status" -eq 0 ] && [ "$main_status" -eq 0 ]; then
	classification="not_reproduced"
elif [ "$reported_status" -ne 0 ] && [ "$main_status" -eq 0 ]; then
	classification="fixed_on_main"
elif [ "$reported_status" -eq 0 ] && [ "$main_status" -ne 0 ]; then
	classification="regression_on_main"
fi

reported_ref_json=$(json_escape "$REPORTED_REF")
reported_version_json=$(json_escape "${REPORTED_VERSION:-}")
cat >"$ARTIFACTS/compare.json" <<EOF
{
  "reportedRef": "${reported_ref_json}",
  "reportedVersion": "${reported_version_json}",
  "reportedSHA": "${reported_sha}",
  "reportedExitCode": ${reported_status},
  "mainExitCode": ${main_status},
  "classification": "${classification}"
}
EOF

case "$classification" in
fixed_on_main)
	exit 0
	;;
still_failing|regression_on_main|not_reproduced)
	exit 1
	;;
esac
