#!/usr/bin/env bash
set -euo pipefail

test -s docs/third-party-notices.md
missing=0

while IFS= read -r module; do
	if ! grep -Fq -- "[$module](" docs/third-party-notices.md; then
		echo "third-party notice is missing for $module" >&2
		missing=1
	fi
done < <(
	go list -m -f '{{if and (not .Main) (not .Indirect)}}{{.Path}}{{end}}' all |
		sed '/^$/d'
)

if (( missing != 0 )); then
	echo "refresh docs/third-party-notices.md before release" >&2
	exit 1
fi

echo "direct dependency notices are present"
