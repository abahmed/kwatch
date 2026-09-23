#!/bin/sh

set -eu

root_dir=${KWATCH_REPO_ROOT:-$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)}
cd "$root_dir"

: "${REPORTED_REF:?REPORTED_REF is required for compare mode}"
: "${ARTIFACTS:=$(mktemp -d)}"
: "${KIND_CLUSTER_NAME:=kwatch-e2e}"
: "${KEEP_CLUSTER:=false}"

reported_root=""
reported_status=2
main_status=2

cleanup() {
	status=$?
	if [ -n "$reported_root" ]; then
		git worktree remove --force "$reported_root" >/dev/null 2>&1 || true
	fi
	exit "$status"
}

trap cleanup EXIT INT TERM

mkdir -p "$ARTIFACTS/reported" "$ARTIFACTS/main"
reported_sha=$(git rev-parse "$REPORTED_REF^{commit}" 2>/dev/null || true)
if [ -z "$reported_sha" ]; then
	git fetch --depth=1 origin "$REPORTED_REF"
	reported_sha=$(git rev-parse 'FETCH_HEAD^{commit}')
fi

reported_root=$(mktemp -d "${TMPDIR:-/tmp}/kwatch-reported.XXXXXX")
rmdir "$reported_root"
git worktree add --detach "$reported_root" "$reported_sha" >/dev/null

for required_file in Dockerfile deploy/crd.yaml deploy/deploy.yaml; do
	if [ ! -f "$reported_root/$required_file" ]; then
		cat >"$ARTIFACTS/compare.json" <<EOF
{
  "reportedRef": "$REPORTED_REF",
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

json_escape() {
	printf '%s' "$1" | sed 's/[\\&]/\\&\\&/g; s/"/\\\\"/g'
}

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
