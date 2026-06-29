#!/usr/bin/env bash
set -euo pipefail

# Helm chart template tests
# Verify that the chart renders correctly for different replicaCounts
# and for the LLM sidecar enable/disable scenarios.
cd "$(dirname "$0")"

echo "=== replicaCount=1 (default) ==="
OUT1=$(helm template test1 . 2>&1)

echo "$OUT1" | grep -q "livenessProbe" || { echo "FAIL: probes missing"; exit 1; }
echo "$OUT1" | grep -q "readinessProbe" || { echo "FAIL: readinessProbe missing"; exit 1; }
echo "$OUT1" | grep -q "replicas: 1" || { echo "FAIL: replicas not 1"; exit 1; }
echo "$OUT1" | grep -q "strategy:" || { echo "FAIL: strategy missing"; exit 1; }
echo "$OUT1" | grep -q "type: Recreate" || { echo "FAIL: strategy not Recreate"; exit 1; }
if echo "$OUT1" | grep -q "coordination.k8s.io"; then
  echo "FAIL: leases RBAC present at replicaCount=1"; exit 1
fi
if echo "$OUT1" | grep -q "leaderElection"; then
  echo "FAIL: leaderElection present"; exit 1
fi
# LLM must be absent by default
if echo "$OUT1" | grep -q "kwatch-llm"; then
  echo "FAIL: LLM sidecar present at default (should be absent)"; exit 1
fi
echo "PASS: replicaCount=1"

echo "=== replicaCount=2 (must fail) ==="
OUT2=$(helm template test2 . --set replicaCount=2 2>&1) && {
  echo "FAIL: expected error but template succeeded"; exit 1
} || true
echo "$OUT2" | grep -q "must run a single replica" || { echo "FAIL: expected replica guard message"; exit 1; }
echo "PASS: replicaCount=2 rejected"

echo "=== memory limit ==="
echo "$OUT1" | grep -q "memory: 256Mi" || { echo "FAIL: memory limit not 256Mi"; exit 1; }
echo "PASS: memory limit"

echo "=== LLM disabled (default) ==="
if echo "$OUT1" | grep -qi "kwatch-llm\|kwatch-triage"; then
  echo "FAIL: LLM artefacts found with LLM disabled"; exit 1
fi
echo "PASS: LLM disabled"

echo "=== LLM enabled (plain container) ==="
OUT3=$(helm template test3 . --set config.llm.enabled=true --set llm.nativeSidecar=false --set replicaCount=1 2>&1)
echo "$OUT3" | grep -q "kwatch-llm" || { echo "FAIL: LLM sidecar missing when enabled"; exit 1; }
echo "$OUT3" | grep -q "/health" || { echo "FAIL: health probe missing"; exit 1; }
echo "$OUT3" | grep -q "8080" || { echo "FAIL: port 8080 missing"; exit 1; }
echo "$OUT3" | grep -A2 "config.yaml:" | grep -q "llm:" || { echo "FAIL: llm: not in ConfigMap"; exit 1; }
echo "$OUT3" | grep -A3 "config.yaml:" | grep -q "enabled: true" || { echo "FAIL: llm.enabled: true not in ConfigMap"; exit 1; }
# Must NOT appear in initContainers (plain container)
if echo "$OUT3" | grep -A3 "initContainers" | grep -q "kwatch-llm"; then
  echo "FAIL: LLM in initContainers with nativeSidecar=false"; exit 1
fi
echo "PASS: LLM enabled (plain container)"

echo "=== LLM enabled (native sidecar) ==="
OUT4=$(helm template test4 . --set config.llm.enabled=true --set llm.nativeSidecar=true --set replicaCount=1 2>&1)
echo "$OUT4" | grep -q "restartPolicy: Always" || { echo "FAIL: restartPolicy Always missing in native sidecar"; exit 1; }
# Security context must be present
echo "$OUT4" | grep -q "runAsNonRoot" || { echo "FAIL: securityContext missing"; exit 1; }
echo "PASS: LLM enabled (native sidecar)"

echo "=== LLM enabled + replicaCount > 1 (must fail) ==="
OUT5=$(helm template test5 . --set config.llm.enabled=true --set replicaCount=2 2>&1) && {
  echo "FAIL: expected error but template succeeded"; exit 1
} || true
echo "PASS: replica guard rejected replicaCount>1"

echo "=== LLM disabled + replicaCount > 1 (must fail due to replica guard) ==="
OUT6=$(helm template test6 . --set replicaCount=2 2>&1) && {
  echo "FAIL: expected error but template succeeded"; exit 1
} || true
echo "PASS: replica guard rejected replicaCount>1 (LLM disabled)"

echo "All helm template tests passed."
