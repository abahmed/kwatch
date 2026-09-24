#!/bin/sh

set -eu

artifacts=${1:?artifact directory is required}
if [ ! -d "$artifacts" ]; then
	echo "artifact directory does not exist: $artifacts" >&2
	exit 2
fi

patterns='KWATCH_E2E_CANARY_|client-certificate-data:|client-key-data:|'
patterns="${patterns}Bearer [^[]|Basic [^[]|e2e-token|kind: Secret|"
patterns="${patterns}diagnostics[_-]*token[=:][^[]|"
patterns="${patterns}password[=:][^[]|api[_-]*key[=:][^[]"
if grep -RIqiE --exclude='*.png' "$patterns" "$artifacts";
then
	echo "unsafe content found in E2E artifacts" >&2
	exit 1
fi
