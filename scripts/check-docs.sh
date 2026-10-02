#!/bin/sh

set -eu
# pipefail is not POSIX; enable it where the shell supports it.
# shellcheck disable=SC3040
(set -o pipefail) 2>/dev/null && set -o pipefail

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
cd "$root_dir"

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

grep -Fq 'state.db' docs/architecture.md || {
  echo "documentation: architecture persistence contract is missing" >&2
  exit 1
}
if grep -Fq 'ConfigMap shard' docs/architecture.md; then
  echo "documentation: stale ConfigMap persistence guidance" >&2
  exit 1
fi

grep -Fq 'required informer caches' docs/configuration.md || {
  echo "documentation: readiness semantics are missing" >&2
  exit 1
}
if grep -Eq '/(incidents|deadletters|test-alert|debug/pprof)|diagnosticsToken' \
  docs/configuration.md; then
  echo "documentation: removed diagnostic endpoints are still documented" >&2
  exit 1
fi

echo "documentation checks passed"
