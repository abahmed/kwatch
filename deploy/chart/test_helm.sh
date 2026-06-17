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
if echo "$OUT1" | grep -q "leaderElection"; then
  echo "FAIL: leaderElection present at replicaCount=1"; exit 1
fi
if echo "$OUT1" | grep -q "coordination.k8s.io"; then
  echo "FAIL: leases RBAC present at replicaCount=1"; exit 1
fi
# LLM must be absent by default
if echo "$OUT1" | grep -q "kwatch-llm"; then
  echo "FAIL: LLM sidecar present at default (should be absent)"; exit 1
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

echo "=== LLM disabled (default) ==="
if echo "$OUT1" | grep -qi "kwatch-llm\|kwatch-triage"; then
  echo "FAIL: LLM artefacts found with LLM disabled"; exit 1
fi
echo "PASS: LLM disabled"

echo "=== LLM enabled (plain container) ==="
OUT3=$(helm template test3 . --set config.llm.enabled=true --set llm.nativeSidecar=false --set replicaCount=1 2>&1)
echo "$OUT3" | grep -q "kwatch-llm" || { echo "FAIL: LLM sidecar missing when enabled"; exit 1; }
echo "$OUT3" | grep -q "OLLAMA_HOST" || { echo "FAIL: OLLAMA_HOST env missing"; exit 1; }
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

echo "=== LLM disabled + replicaCount > 1 (must succeed) ==="
OUT6=$(helm template test6 . --set replicaCount=2 2>&1)
echo "$OUT6" | grep -q "replicas: 2" || { echo "FAIL: replicas not 2"; exit 1; }
echo "$OUT6" | grep -q "leaderElection" || { echo "FAIL: leaderElection missing"; exit 1; }
if echo "$OUT6" | grep -q "kwatch-llm"; then
  echo "FAIL: LLM sidecar present when disabled at replicaCount=2"; exit 1
fi
echo "PASS: LLM disabled + replicaCount>1"

echo "All helm template tests passed."
