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

# One writer with a Lease lock and a persistent state volume.
require 'replicas: 1' "raw deployment must run one replica"
require 'type: Recreate' "single-writer deployment must use Recreate"
require 'path: /availabilityz' "deployment availability probe is missing"
require 'terminationGracePeriodSeconds: 60' \
	"shutdown grace must be at least 60 seconds"
require 'name: kwatch-leader-election' "Lease RBAC is missing"
require 'name: kwatch-restart-evidence' "own-namespace pod get is missing"
require '  resources: ["namespaces", "nodes"]' "node get is missing"
require 'readOnlyRootFilesystem: true' "read-only filesystem is missing"
require 'kind: PersistentVolumeClaim' "state volume claim is missing"
require 'mountPath: /var/lib/kwatch' "state volume is not mounted"
require 'tolerationSeconds: 30' "fast node-failure tolerations are missing"
# The raw manifest uses rbac.mode=full: list and watch on every resource,
# and get only on the few objects kwatch reads one at a time. A wildcard
# get would also match subresources such as pods/exec and nodes/proxy.
require '  resources: ["*"]' "raw ClusterRole must use full list access"
require '  verbs: ["list", "watch"]' "raw ClusterRole must be list/watch only"
if grep -Fq 'verbs: ["get", "list", "watch"]' "$manifest"; then
	echo "manifest parity: raw ClusterRole must not use wildcard get" >&2
	exit 1
fi
# Kubelet stats are read from each kubelet directly, never through the
# API server proxy.
require '  resources: ["nodes/stats", "nodes/metrics", "pods/log"]' \
	"kubelet stats RBAC is missing"
if grep -Fq '"nodes/proxy"' "$manifest"; then
	echo "manifest parity: nodes/proxy must not be granted" >&2
	exit 1
fi
# Get and update only on kwatch's own Lease.
require '  resourceNames: ["kwatch-leader"]' \
	"Lease get/update must be limited to the Lease name"
require 'karpenter.sh/do-not-disrupt: "true"' \
	"pod must ask node autoscalers not to disrupt it"
require 'cluster-autoscaler.kubernetes.io/safe-to-evict: "false"' \
	"pod must ask the cluster autoscaler not to evict it"
require 'memory: "512Mi"' "memory limit must be 512Mi"
require 'cpu: "500m"' "cpu limit must be 500m (matches the chart)"
# Both probes need a timeout that survives a busy pod; the chart sets 3s.
probe_timeouts=$(grep -c 'timeoutSeconds: 3' "$manifest" || true)
if [ "$probe_timeouts" -lt 2 ]; then
	echo "manifest parity: liveness and readiness need timeoutSeconds: 3" >&2
	exit 1
fi
require 'name: KWATCH_MEMORY_LIMIT' "memory limit is not passed to kwatch"
if grep -Fq 'name: GOMEMLIMIT' "$manifest"; then
	echo "manifest parity: GOMEMLIMIT is derived from the limit" >&2
	exit 1
fi
if grep -Fq 'kind: PodDisruptionBudget' "$manifest"; then
	echo "manifest parity: a single-writer deployment must not have a PDB" >&2
	exit 1
fi

echo "manifest parity: PASS"
