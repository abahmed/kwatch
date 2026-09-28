#!/bin/sh

set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
cd "$root_dir"

manifest=deploy/deploy.yaml

require() {
	grep -Fq "$1" "$manifest" || {
		echo "manifest parity: $2" >&2
		exit 1
	}
}

# ADR 0010: one writer with a Lease lock and a persistent state volume.
require 'replicas: 1' "raw deployment must run one replica"
require 'type: Recreate' "single-writer deployment must use Recreate"
require 'path: /availabilityz' "deployment availability probe is missing"
require 'terminationGracePeriodSeconds: 60' \
	"shutdown grace must be at least 60 seconds"
require 'name: kwatch-leader-election' "Lease RBAC is missing"
require 'readOnlyRootFilesystem: true' "read-only filesystem is missing"
require 'kind: PersistentVolumeClaim' "state volume claim is missing"
require 'mountPath: /var/lib/kwatch' "state volume is not mounted"
require 'tolerationSeconds: 30' "fast node-failure tolerations are missing"
if grep -Fq 'kind: PodDisruptionBudget' "$manifest"; then
	echo "manifest parity: a single-writer deployment must not have a PDB" >&2
	exit 1
fi

echo "manifest parity: PASS"
