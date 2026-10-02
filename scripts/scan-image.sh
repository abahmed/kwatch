#!/usr/bin/env bash
set -euo pipefail

# scan-image.sh IMAGE
#
# Fails on HIGH or CRITICAL findings in a local or remote image. OS packages
# are scanned with --ignore-unfixed, because a base-layer CVE without a fixed
# package cannot be remediated here; Go modules are always reported. Time-
# bounded exceptions live in .trivyignore (docs/vulnerability-exceptions.md).
# A local trivy binary is used when present, otherwise the pinned image.
image=${1:?usage: scan-image.sh IMAGE}
root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
trivy_image=aquasec/trivy@sha256:\
33f816d414b9d582d25bb737ffa4a632ae34e222f7ec1b50252cef0ce2266006
cache_dir=${TRIVY_CACHE_DIR:-${HOME}/.cache/trivy}

run_trivy() {
	if command -v trivy >/dev/null 2>&1; then
		trivy image --cache-dir "$cache_dir" \
			--ignorefile "$root_dir/.trivyignore" "$@"
		return
	fi
	if ! command -v docker >/dev/null 2>&1; then
		echo "trivy or docker is required to scan $image" >&2
		exit 1
	fi
	mkdir -p "$cache_dir"
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "$cache_dir:/root/.cache/trivy" \
		-v "$root_dir/.trivyignore:/kwatch/.trivyignore:ro" \
		"$trivy_image" image --ignorefile /kwatch/.trivyignore "$@"
}

run_trivy --severity HIGH,CRITICAL --exit-code 1 \
	--pkg-types os --ignore-unfixed "$image"
run_trivy --severity HIGH,CRITICAL --exit-code 1 \
	--pkg-types library "$image"
