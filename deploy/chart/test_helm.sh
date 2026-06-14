#!/usr/bin/env bash
set -euo pipefail

# Helm chart template tests
# Verify that the chart renders correctly for different replicaCounts.
cd "$(dirname "$0")"

echo "=== replicaCount=1 (default) ==="
OUT1=$(helm template test1 . 2>&1)

echo "$OUT1" | grep -q "livenessProbe" || { echo "FAIL: probes missing"; exit 1; }
echo "$OUT1" | grep -q "readinessProbe" || { echo "FAIL: readinessProbe missing"; exit 1; }
echo "$OUT1" | grep -q "replicas: 1" || { echo "FAIL: replicas not 1"; exit 1; }
if echo "$OUT1" | grep -q "leaderElection"; then
  echo "FAIL: leaderElection present at replicaCount=1"; exit 1
fi
if echo "$OUT1" | grep -q "coordination.k8s.io"; then
  echo "FAIL: leases RBAC present at replicaCount=1"; exit 1
fi
echo "PASS: replicaCount=1"

echo "=== replicaCount=2 ==="
OUT2=$(helm template test2 . --set replicaCount=2 2>&1)

echo "$OUT2" | grep -q "leaderElection" || { echo "FAIL: leaderElection missing at replicaCount=2"; exit 1; }
echo "$OUT2" | grep -q "coordination.k8s.io" || { echo "FAIL: leases RBAC missing at replicaCount=2"; exit 1; }
echo "$OUT2" | grep -q "replicas: 2" || { echo "FAIL: replicas not 2"; exit 1; }
echo "PASS: replicaCount=2"

echo "=== memory limit ==="
echo "$OUT1" | grep -q "memory: 256Mi" || { echo "FAIL: memory limit not 256Mi"; exit 1; }
echo "PASS: memory limit"

echo "All helm template tests passed."
