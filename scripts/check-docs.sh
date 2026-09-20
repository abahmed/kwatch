#!/bin/sh

set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
cd "$root_dir"

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

find docs/adr -maxdepth 1 -type f -name '*.md' \
  -exec basename {} \; |
  sed -n 's/^\([0-9][0-9][0-9][0-9]\)-.*/\1/p' |
  sort | uniq -d >"$tmp_dir/duplicate-adr-numbers"
if test -s "$tmp_dir/duplicate-adr-numbers"; then

  echo "documentation: duplicate ADR numbers" >&2
  cat "$tmp_dir/duplicate-adr-numbers" >&2
  exit 1
fi

grep -Fq 'ConfigureSources' docs/architecture.md || {
  echo "documentation: architecture source contract is missing" >&2
  exit 1
}
if grep -Fq "optional wiring uses \`Set<Type>\`" docs/architecture.md; then
  echo "documentation: stale mutable source-wiring guidance" >&2
  exit 1
fi

grep -Fq 'required informer caches' docs/configuration.md || {
  echo "documentation: readiness semantics are missing" >&2
  exit 1
}
grep -Fq 'production validation requires' docs/configuration.md || {
  echo "documentation: diagnostic authentication guidance is missing" >&2
  exit 1
}

echo "documentation checks passed"
