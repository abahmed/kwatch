#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
# shellcheck source=require-command.sh
. "$script_dir/require-command.sh"
require_command rg

# Test files are split by behavior or subsystem, never by an arbitrary part
# number or an "extra" catch-all. This keeps the package tree searchable.
matches=$(rg --files -g '*_test.go' | rg \
  '(^|/)[^/]*(part[0-9]+|extra)[^/]*_test\.go$' || true)

if [ -n "$matches" ]; then
	echo "test layout violation: use responsibility-based test names"
	echo "$matches"
	exit 1
fi

echo "test layout check passed"
