#!/bin/sh

set -eu

root_dir=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root_dir"

manifest=deploy/deploy.yaml

grep -Fq 'replicas: 2' "$manifest" || {
	echo "manifest parity: raw deployment must default to two replicas" >&2
	exit 1
}
grep -Fq 'maxUnavailable: 0%' "$manifest" || {
	echo "manifest parity: rolling update availability policy is missing" >&2
	exit 1
}
grep -Fq 'path: /availabilityz' "$manifest" || {
	echo "manifest parity: deployment availability probe is missing" >&2
	exit 1
}
grep -Fq 'terminationGracePeriodSeconds: 60' "$manifest" || {
	echo "manifest parity: shutdown grace must be at least 60 seconds" >&2
	exit 1
}
grep -Fq 'kind: PodDisruptionBudget' "$manifest" || {
	echo "manifest parity: multi-replica deployment needs a PDB" >&2
	exit 1
}
grep -Fq 'name: kwatch-leader-election' "$manifest" || {
	echo "manifest parity: Lease RBAC is missing" >&2
	exit 1
}
grep -Fq 'topologyKey: kubernetes.io/hostname' "$manifest" || {
	echo "manifest parity: pod placement spreading is missing" >&2
	exit 1
}
grep -Fq 'readOnlyRootFilesystem: true' "$manifest" || {
	echo "manifest parity: read-only filesystem is missing" >&2
	exit 1
}

echo "manifest parity: PASS"
