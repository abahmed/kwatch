#!/usr/bin/env bash
set -euo pipefail

# Helm chart template tests
cd "$(dirname "$0")"

echo "=== default template ==="
OUT1=$(helm template test1 . 2>&1)

grep -Fq "livenessProbe" <<<"$OUT1" || {
  echo "FAIL: probes missing"; exit 1;
}
grep -Fq "readinessProbe" <<<"$OUT1" || {
  echo "FAIL: readinessProbe missing"; exit 1;
}
grep -Fq "replicas: 1" <<<"$OUT1" || {
  echo "FAIL: deployment must run one replica"; exit 1;
}
grep -Fq "type: Recreate" <<<"$OUT1" || {
  echo "FAIL: deployment strategy must be Recreate"; exit 1;
}
grep -Fq "path: /availabilityz" <<<"$OUT1" || {
  echo "FAIL: deployment availability probe is missing"; exit 1;
}
grep -Fq "terminationGracePeriodSeconds: 60" <<<"$OUT1" || {
  echo "FAIL: termination grace period is not configured"; exit 1;
}
if grep -Fq "kind: PodDisruptionBudget" <<<"$OUT1"; then
  echo "FAIL: a single-writer deployment must not have a PDB"; exit 1;
fi
grep -Fq "kind: PersistentVolumeClaim" <<<"$OUT1" || {
  echo "FAIL: state volume claim is missing"; exit 1;
}
grep -Fq "mountPath: /var/lib/kwatch" <<<"$OUT1" || {
  echo "FAIL: state volume is not mounted"; exit 1;
}
echo "PASS: default"

echo "=== evaluation without storage ==="
OUT_EPHEMERAL=$(helm template test-ephemeral . \
  --set persistence.emptyDir=true 2>&1)
if grep -Fq "kind: PersistentVolumeClaim" <<<"$OUT_EPHEMERAL"; then
  echo "FAIL: emptyDir mode must not create a claim"; exit 1;
fi
grep -Fq "emptyDir: {}" <<<"$OUT_EPHEMERAL" || {
  echo "FAIL: emptyDir volume missing"; exit 1;
}
echo "PASS: evaluation without storage"

echo "=== memory limit ==="
grep -Fq "memory: 256Mi" <<<"$OUT1" || {
  echo "FAIL: memory limit not 256Mi"; exit 1;
}
echo "PASS: memory limit"

echo "=== security context ==="
grep -Fq "runAsNonRoot" <<<"$OUT1" || {
  echo "FAIL: securityContext missing"; exit 1;
}
grep -Fq "allowPrivilegeEscalation: false" <<<"$OUT1" || {
  echo "FAIL: privilege escalation is not disabled"; exit 1;
}
grep -Fq "type: RuntimeDefault" <<<"$OUT1" || {
  echo "FAIL: seccomp profile missing"; exit 1;
}
grep -Fq "resourceNames:" <<<"$OUT1" || {
  echo "FAIL: state ConfigMap RBAC is not restricted"; exit 1;
}
grep -Fq '  - "kwatch-telemetry"' <<<"$OUT1" || {
	echo "FAIL: telemetry ConfigMap RBAC is missing"; exit 1;
}
grep -Fq 'apiGroups: ["coordination.k8s.io"]' <<<"$OUT1" || {
	echo "FAIL: Lease RBAC is missing"; exit 1;
}
grep -Fq 'resources: ["leases"]' <<<"$OUT1" || {
	echo "FAIL: Lease resource RBAC is missing"; exit 1;
}
grep -Fq 'value: "test1-kwatch-leader"' <<<"$OUT1" || {
	echo "FAIL: deterministic Lease identity is missing"; exit 1;
}
echo "PASS: security context"

echo "=== optional network policy ==="
if grep -Fq "kind: NetworkPolicy" <<<"$OUT1"; then
  echo "FAIL: network policy is enabled by default"; exit 1
fi
OUT2=$(helm template test2 . --set networkPolicy.enabled=true 2>&1)
grep -Fq "kind: NetworkPolicy" <<<"$OUT2" || {
  echo "FAIL: enabled network policy was not rendered"; exit 1;
}
echo "PASS: optional network policy"

cmp -s ../crd.yaml templates/kwatchconfig-crd.yaml || {
  echo "FAIL: chart CRD is out of sync with deploy/crd.yaml"
  exit 1
}
echo "PASS: CRD is in sync"

grep -Fq "kind: CustomResourceDefinition" <<<"$OUT1" || {
  echo "FAIL: CRD is not rendered by the chart"
  exit 1
}
echo "PASS: CRD is rendered"

echo "All helm template tests passed."
