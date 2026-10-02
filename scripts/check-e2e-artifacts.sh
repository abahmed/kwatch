#!/bin/sh

set -eu

artifacts=${1:?artifact path is required}
if [ ! -e "$artifacts" ]; then
	echo "artifact path does not exist: $artifacts" >&2
	exit 2
fi

patterns='KWATCH_E2E_CANARY_|client-certificate-data:|client-key-data:|'
patterns="${patterns}Bearer [^[]|Basic [^[]|e2e-token|kind: Secret|"
patterns="${patterns}diagnostics[_-]*token[=:][^[]|"
patterns="${patterns}password[=:][^[]|api[_-]*key[=:][^[]"
status=0
grep -RIqiE --exclude='*.png' "$patterns" "$artifacts" || status=$?
case "$status" in
0)
	echo "unsafe content found in E2E artifacts" >&2
	exit 1
	;;
1)
	;;
*)
	echo "scan error: grep exited with status $status" >&2
	exit 1
	;;
esac
