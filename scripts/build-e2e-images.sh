#!/bin/sh

# Build the three images the Kind scenarios use: Kwatch, the webhook
# receiver and the workload. They are built side by side. Names carry the
# source commit, so a build in one CI job can be loaded in another.
#
# Usage: build-e2e-images.sh [SAVE_FILE]
#   SAVE_FILE  also write the images to this gzip tar (docker save).
#
# Environment: KWATCH_CANDIDATE_ROOT (source to build, default: this repo),
# KWATCH_SOURCE_SHA (default: git rev-parse HEAD), BUILD_DATE,
# BUILD_KWATCH_IMAGE=false to skip the Kwatch image (a custom one is used).
set -eu

default_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
harness_root=${KWATCH_HARNESS_ROOT:-${KWATCH_REPO_ROOT:-$default_root}}
candidate_root=${KWATCH_CANDIDATE_ROOT:-$harness_root}
cd "$harness_root"

: "${BUILD_DATE:=$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
source_sha=${KWATCH_SOURCE_SHA:-$(git rev-parse HEAD)}
kwatch_image="kwatch:e2e-${source_sha}"
receiver_image="kwatch-e2e-receiver:${source_sha}"
workload_image="kwatch-e2e-workload:${source_sha}"

kwatch_pid=""
if [ "${BUILD_KWATCH_IMAGE:-true}" = true ]; then
	docker build --load --tag "$kwatch_image" \
		--build-arg RELEASE_VERSION=e2e \
		--build-arg GIT_COMMIT="$source_sha" \
		--build-arg BUILD_DATE="$BUILD_DATE" "$candidate_root" &
	kwatch_pid=$!
fi
docker build --load --tag "$receiver_image" test/e2e/receiver &
receiver_pid=$!
docker build --load --tag "$workload_image" test/e2e/workload &
workload_pid=$!

failed=false
for pid in $kwatch_pid $receiver_pid $workload_pid; do
	wait "$pid" || failed=true
done
if [ "$failed" = true ]; then
	echo "an e2e image failed to build" >&2
	exit 1
fi

if [ -n "${1:-}" ]; then
	docker save "$kwatch_image" "$receiver_image" "$workload_image" |
		gzip -1 >"$1"
fi
