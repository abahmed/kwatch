#!/usr/bin/env bash
set -euo pipefail

# This script deliberately fails when a requested scanner is unavailable. A
# skipped security scan must not look like a successful release check.
go mod verify
./scripts/check-third-party-notices.sh

vulncheck="${GOVULNCHECK_BIN:-govulncheck}"
if ! command -v "$vulncheck" >/dev/null 2>&1 &&
	[[ -x "$(go env GOPATH)/bin/govulncheck" ]]; then
	vulncheck="$(go env GOPATH)/bin/govulncheck"
fi
if ! command -v "$vulncheck" >/dev/null 2>&1 &&
	[[ ! -x "$vulncheck" ]]; then
	echo "govulncheck is required for dependency scanning" >&2
	exit 1
fi
"$vulncheck" ./...

if [[ -n "${KWATCH_IMAGE:-}" ]]; then
  if ! command -v trivy >/dev/null 2>&1; then
    echo "trivy is required when KWATCH_IMAGE is set" >&2
    exit 1
  fi
  trivy image --severity HIGH,CRITICAL --exit-code 1 "$KWATCH_IMAGE"
else
  echo "KWATCH_IMAGE is not set; image scanning is deferred to release CI"
fi
