#!/bin/sh

set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
cd "$root_dir"

: "${REPORTED_IMAGE:?REPORTED_IMAGE is required for comparison}"
: "${ARTIFACTS:=$(mktemp -d)}"
: "${SCENARIO_REGEX:=TestScenario}"
: "${SUITE_TIMEOUT:=60m}"
: "${SOURCE_REF:=main}"
: "${REPORTED_SOURCE_REF:=}"
: "${KWATCH_CONFIG_FILE:=test/e2e/testdata/base-config.yaml}"

reported_artifacts="$ARTIFACTS/reported"
main_artifacts="$ARTIFACTS/main"
mkdir -p "$reported_artifacts" "$main_artifacts"

pull_cleanup=false
cleanup() {
	status=${1:-$?}
	trap - EXIT INT TERM
	if [ "$pull_cleanup" = true ]; then
		docker image rm "$REPORTED_IMAGE" >/dev/null 2>&1 || true
	fi
	exit "$status"
}
trap cleanup EXIT INT TERM

printf '%s\n' \
	"reported source ref is metadata only; using current main manifests" \
	>"$ARTIFACTS/reported-source-warning.txt"

command -v docker >/dev/null 2>&1 || {
	echo "required tool is missing: docker" >&2
	exit 2
}

if ! docker image inspect "$REPORTED_IMAGE" >/dev/null 2>&1; then
	docker pull "$REPORTED_IMAGE"
	pull_cleanup=true
fi

run_version() {
	version_name=$1
	cluster_name=$2
	image=$3
	artifact_dir=$4
	set +e
	KIND_CLUSTER_NAME="$cluster_name" \
	ARTIFACTS="$artifact_dir" \
	KWATCH_IMAGE="$image" \
	SCENARIO_REGEX="$SCENARIO_REGEX" \
	KWATCH_CONFIG_FILE="$KWATCH_CONFIG_FILE" \
	KWATCH_REPO_ROOT="$5" \
	KWATCH_SOURCE_SHA="$6" \
	ISSUE_FIXTURE_DIR="${ISSUE_FIXTURE_DIR:-}" \
	ISSUE_EXPECTED_REASON="${ISSUE_EXPECTED_REASON:-}" \
	ISSUE_NAMESPACE="${ISSUE_NAMESPACE:-}" \
	SUITE_TIMEOUT="$SUITE_TIMEOUT" \
	KEEP_CLUSTER=false \
	SOURCE_REF="$SOURCE_REF" \
	./scripts/test-kind-scenarios.sh
	version_status=$?
	set -e
	printf '%s\n' "$version_status" >"$artifact_dir/exit-code"
	printf '%s\n' "$version_name" >"$artifact_dir/version"
	return 0
}

run_version reported "kwatch-e2e-reported-$$" \
	"$REPORTED_IMAGE" "$reported_artifacts" "$root_dir" ""
run_version main "kwatch-e2e-main-$$" \
	kwatch:e2e "$main_artifacts" "$root_dir" ""

reported_status=$(sed -n '1p' "$reported_artifacts/exit-code")
main_status=$(sed -n '1p' "$main_artifacts/exit-code")
classification=not_reproduced
if [ "$reported_status" -ne 0 ] && [ "$main_status" -eq 0 ]; then
	classification=fixed_on_main
elif [ "$reported_status" -ne 0 ] && [ "$main_status" -ne 0 ]; then
	classification=still_failing
elif [ "$reported_status" -eq 0 ] && [ "$main_status" -ne 0 ]; then
	classification=regression_on_main
fi

cat >"$ARTIFACTS/comparison.json" <<EOF
{
  "reportedImage": "$REPORTED_IMAGE",
  "reportedVersion": "${REPORTED_VERSION:-unknown}",
  "reportedSourceRef": "${REPORTED_SOURCE_REF:-unknown}",
  "mainRef": "$SOURCE_REF",
  "reportedExitCode": $reported_status,
  "mainExitCode": $main_status,
  "classification": "$classification"
}
EOF
cat "$ARTIFACTS/comparison.json"

if [ "$classification" = fixed_on_main ] ||
	[ "$classification" = not_reproduced ]; then
	cleanup 0
fi
cleanup 1
