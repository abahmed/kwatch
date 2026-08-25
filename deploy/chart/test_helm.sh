#!/usr/bin/env bash
set -euo pipefail

# Helm chart template tests
cd "$(dirname "$0")"

echo "=== default template ==="
OUT1=$(helm template test1 . 2>&1)

echo "$OUT1" | grep -q "livenessProbe" || { echo "FAIL: probes missing"; exit 1; }
echo "$OUT1" | grep -q "readinessProbe" || { echo "FAIL: readinessProbe missing"; exit 1; }
echo "$OUT1" | grep -q "replicas: 1" || { echo "FAIL: replicas not 1"; exit 1; }
echo "$OUT1" | grep -q "strategy:" || { echo "FAIL: strategy missing"; exit 1; }
echo "$OUT1" | grep -q "type: Recreate" || { echo "FAIL: strategy not Recreate"; exit 1; }
echo "PASS: default"

echo "=== memory limit ==="
echo "$OUT1" | grep -q "memory: 256Mi" || { echo "FAIL: memory limit not 256Mi"; exit 1; }
echo "PASS: memory limit"

echo "=== security context ==="
echo "$OUT1" | grep -q "runAsNonRoot" || { echo "FAIL: securityContext missing"; exit 1; }
echo "PASS: security context"

echo "All helm template tests passed."
