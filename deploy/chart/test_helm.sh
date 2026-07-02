#!/usr/bin/env bash
set -euo pipefail

# Helm chart template tests
# Verify that the chart renders correctly and for LLM enable/disable scenarios.
cd "$(dirname "$0")"

echo "=== default template ==="
OUT1=$(helm template test1 . 2>&1)

echo "$OUT1" | grep -q "livenessProbe" || { echo "FAIL: probes missing"; exit 1; }
echo "$OUT1" | grep -q "readinessProbe" || { echo "FAIL: readinessProbe missing"; exit 1; }
echo "$OUT1" | grep -q "replicas: 1" || { echo "FAIL: replicas not 1"; exit 1; }
echo "$OUT1" | grep -q "strategy:" || { echo "FAIL: strategy missing"; exit 1; }
echo "$OUT1" | grep -q "type: Recreate" || { echo "FAIL: strategy not Recreate"; exit 1; }
# LLM must be absent by default
if echo "$OUT1" | grep -q "kwatch-llm"; then
  echo "FAIL: LLM sidecar present at default (should be absent)"; exit 1
fi
echo "PASS: default"

echo "=== memory limit ==="
echo "$OUT1" | grep -q "memory: 256Mi" || { echo "FAIL: memory limit not 256Mi"; exit 1; }
echo "PASS: memory limit"

echo "=== LLM disabled (default) ==="
if echo "$OUT1" | grep -qi "kwatch-llm\|kwatch-triage"; then
  echo "FAIL: LLM artefacts found with LLM disabled"; exit 1
fi
echo "PASS: LLM disabled"

echo "=== LLM enabled ==="
OUT3=$(helm template test3 . --set config.llm.enabled=true 2>&1)
echo "$OUT3" | grep -q "kwatch-llm" || { echo "FAIL: LLM sidecar missing when enabled"; exit 1; }
echo "$OUT3" | grep -q "/health" || { echo "FAIL: health probe missing"; exit 1; }
echo "$OUT3" | grep -q "8080" || { echo "FAIL: port 8080 missing"; exit 1; }
echo "$OUT3" | grep -A2 "config.yaml:" | grep -q "llm:" || { echo "FAIL: llm: not in ConfigMap"; exit 1; }
echo "$OUT3" | grep -A3 "config.yaml:" | grep -q "enabled: true" || { echo "FAIL: llm.enabled: true not in ConfigMap"; exit 1; }
# Security context must be present
echo "$OUT3" | grep -q "runAsNonRoot" || { echo "FAIL: securityContext missing"; exit 1; }
echo "PASS: LLM enabled"

echo "All helm template tests passed."
