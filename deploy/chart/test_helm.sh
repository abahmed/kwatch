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

echo "=== KwatchConfig overlay with configSecretName ==="
# config.yaml in an operator Secret never sees config.crd.enabled, so the
# chart passes it as KWATCH_CRD_ENABLED.
OUT_CRD_SECRET=$(helm template test-crd . --set configSecretName=kwatch-config 2>&1)
grep -A1 -F "name: KWATCH_CRD_ENABLED" <<<"$OUT_CRD_SECRET" |
  grep -Fq 'value: "true"' || {
  echo "FAIL: configSecretName must keep the CRD overlay enabled"; exit 1;
}
OUT_CRD_OFF=$(helm template test-crd . --set configSecretName=kwatch-config \
  --set config.crd.enabled=false 2>&1)
grep -A1 -F "name: KWATCH_CRD_ENABLED" <<<"$OUT_CRD_OFF" |
  grep -Fq 'value: "false"' || {
  echo "FAIL: config.crd.enabled=false must reach kwatch"; exit 1;
}
echo "PASS: CRD overlay setting"

echo "=== evaluation without storage ==="
OUT_EPHEMERAL=$(helm template test-ephemeral . \
  --set persistence.emptyDir=true 2>&1)
if grep -Fq "kind: PersistentVolumeClaim" <<<"$OUT_EPHEMERAL"; then
  echo "FAIL: emptyDir mode must not create a claim"; exit 1;
fi
grep -Fq "emptyDir:" <<<"$OUT_EPHEMERAL" || {
  echo "FAIL: emptyDir volume missing"; exit 1;
}
grep -Fq "sizeLimit: \"2Gi\"" <<<"$OUT_EPHEMERAL" || {
  echo "FAIL: emptyDir sizeLimit missing"; exit 1;
}
grep -Fq "name: KWATCH_VOLUME_LIMIT" <<<"$OUT_EPHEMERAL" || {
  echo "FAIL: emptyDir mode must export KWATCH_VOLUME_LIMIT"; exit 1;
}
if grep -Fq "KWATCH_VOLUME_LIMIT" <<<"$OUT1"; then
  echo "FAIL: PVC mode must not set KWATCH_VOLUME_LIMIT"; exit 1;
fi
if helm template test-two-volumes . --set persistence.emptyDir=true \
  --set persistence.existingClaim=state >/dev/null 2>&1; then
  echo "FAIL: emptyDir and existingClaim together must fail"; exit 1;
fi
echo "PASS: evaluation without storage"

echo "=== values schema ==="
for bad in "unknownTopLevel=1" "rbac.bogus=1" "watch.bogus=1" \
  "persistence.bogus=1" "networkPolicy.bogus=1" \
  "config.healthCheck.port=0" "config.healthCheck.port=70000" \
  "config.kubelet.bogus=1" "config.watch.bogus=1"; do
  if helm template test-schema . --set "$bad" >/dev/null 2>&1; then
    echo "FAIL: values schema accepted $bad"; exit 1;
  fi
done
helm template test-schema . --set config.kubelet.insecureSkipVerify=true \
  --set config.watch.secrets=true >/dev/null || {
  echo "FAIL: values schema rejects config.kubelet or config.watch"; exit 1;
}
echo "PASS: values schema"

echo "=== custom tolerations keep failover defaults ==="
OUT_TOL=$(helm template test-tol . \
  --set 'tolerations[0].key=dedicated' \
  --set 'tolerations[0].operator=Exists' 2>&1)
grep -Fq "key: dedicated" <<<"$OUT_TOL" || {
  echo "FAIL: custom toleration missing"; exit 1;
}
test "$(grep -c 'tolerationSeconds: 30' <<<"$OUT_TOL")" = 2 || {
  echo "FAIL: default failover tolerations dropped"; exit 1;
}
OUT_TOL_EFFECT=$(helm template test-tol-effect . \
  --set 'tolerations[0].key=node.kubernetes.io/not-ready' \
  --set 'tolerations[0].operator=Exists' \
  --set 'tolerations[0].effect=NoSchedule' 2>&1)
test "$(grep -c 'tolerationSeconds: 30' <<<"$OUT_TOL_EFFECT")" = 2 || {
  echo "FAIL: a NoSchedule toleration dropped the NoExecute default"; exit 1;
}
echo "PASS: custom tolerations"

echo "=== health port drives container and probe ports ==="
grep -Fq "containerPort: 8060" <<<"$OUT1" || {
  echo "FAIL: default health port is not 8060"; exit 1;
}
OUT_PORT=$(helm template test-port . --set config.healthCheck.port=9090 2>&1)
grep -Fq "containerPort: 9090" <<<"$OUT_PORT" || {
  echo "FAIL: container port does not follow config.healthCheck.port"; exit 1;
}
test "$(grep -c 'port: health' <<<"$OUT_PORT")" = 2 || {
  echo "FAIL: probes must use the named health port"; exit 1;
}
if helm template test-nohealth . \
  --set config.healthCheck.enabled=false >/dev/null 2>&1; then
  echo "FAIL: a disabled health check must fail rendering"; exit 1;
fi
echo "PASS: health port"

echo "=== memory limit ==="
grep -Fq "memory: 512Mi" <<<"$OUT1" || {
  echo "FAIL: memory limit not 512Mi"; exit 1;
}
grep -Fq "name: KWATCH_MEMORY_LIMIT" <<<"$OUT1" || {
  echo "FAIL: memory limit is not passed to kwatch"; exit 1;
}
if grep -Fq "name: GOMEMLIMIT" <<<"$OUT1"; then
  echo "FAIL: GOMEMLIMIT must be derived (90%), not the full limit"; exit 1;
fi
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
if grep -Fq "configmap-manager" <<<"$OUT1"; then
  echo "FAIL: state lives on disk; no ConfigMap write access"; exit 1;
fi
grep -Fq 'apiGroups: ["coordination.k8s.io"]' <<<"$OUT1" || {
	echo "FAIL: Lease RBAC is missing"; exit 1;
}
grep -Fq 'resources: ["leases"]' <<<"$OUT1" || {
	echo "FAIL: Lease resource RBAC is missing"; exit 1;
}
grep -Fq 'value: "test1-kwatch-leader"' <<<"$OUT1" || {
	echo "FAIL: deterministic Lease identity is missing"; exit 1;
}
grep -Fq 'resourceNames: ["test1-kwatch-leader"]' <<<"$OUT1" || {
	echo "FAIL: Lease get/update must be limited to the Lease name"; exit 1;
}
if grep -A6 'kind: ServiceAccount' <<<"$OUT1" | grep -Fq "checksum/"; then
  echo "FAIL: the ServiceAccount must not carry a config checksum"; exit 1;
fi
echo "PASS: security context"

echo "=== optional network policy ==="
if grep -Fq "kind: NetworkPolicy" <<<"$OUT1"; then
  echo "FAIL: network policy is enabled by default"; exit 1
fi
OUT2=$(helm template test2 . --set networkPolicy.enabled=true 2>&1)
grep -Fq "kind: NetworkPolicy" <<<"$OUT2" || {
  echo "FAIL: enabled network policy was not rendered"; exit 1;
}
for want in "port: 8060" "port: 53" "port: 443" "port: 6443" \
  "port: 10250"; do
  grep -Fq "$want" <<<"$OUT2" || {
    echo "FAIL: network policy defaults miss $want"; exit 1;
  }
done
OUT_NP_CIDR=$(helm template test-np . --set networkPolicy.enabled=true \
  --set 'networkPolicy.kubeletCIDRs[0]=10.0.0.0/16' 2>&1)
grep -Fq "cidr: 10.0.0.0/16" <<<"$OUT_NP_CIDR" || {
  echo "FAIL: kubelet egress ignores kubeletCIDRs"; exit 1;
}
OUT_NP_EXTRA=$(helm template test-np . --set networkPolicy.enabled=true \
  --set 'networkPolicy.extraEgressPorts[0]=587' 2>&1)
grep -Fq "port: 587" <<<"$OUT_NP_EXTRA" || {
  echo "FAIL: network policy ignores extraEgressPorts"; exit 1;
}
if grep -Fq "port: 587" <<<"$OUT2"; then
  echo "FAIL: SMTP egress must not be open by default"; exit 1;
fi
echo "PASS: optional network policy"

echo "=== rbac modes ==="
grep -Fq 'resources: ["*"]' <<<"$OUT1" || {
  echo "FAIL: default rbac.mode must be full"; exit 1;
}
OUT_LP=$(helm template test-lp . --set rbac.mode=least-privilege 2>&1)
if grep -Fq 'resources: ["*"]' <<<"$OUT_LP"; then
  echo "FAIL: least-privilege must not use wildcard resources"; exit 1;
fi
for kind in controllerrevisions clusterrolebindings customresourcedefinitions; do
  grep -Fq "$kind" <<<"$OUT_LP" || {
    echo "FAIL: least-privilege is missing $kind"; exit 1;
  }
done
if helm template test-bad . --set rbac.mode=bogus >/dev/null 2>&1; then
  echo "FAIL: invalid rbac.mode was accepted"; exit 1;
fi
for out in "$OUT1" "$OUT_LP"; do
  grep -Fq '"nodes/stats", "nodes/metrics"' <<<"$out" || {
    echo "FAIL: kubelet stats RBAC is missing"; exit 1;
  }
  if grep -Fq '"nodes/proxy"' <<<"$out"; then
    echo "FAIL: nodes/proxy must not be granted"; exit 1;
  fi
done
# ClusterRole and binding are cluster-scoped, so the namespace is part of
# their name: the same release name in two namespaces must not collide.
OUT_A=$(helm template same . --namespace team-a 2>&1)
OUT_B=$(helm template same . --namespace team-b 2>&1)
for pair in "team-a:$OUT_A" "team-b:$OUT_B"; do
  ns=${pair%%:*}
  out=${pair#*:}
  test "$(grep -c "name: same-kwatch-$ns\$" <<<"$out")" -ge 3 || {
    echo "FAIL: ClusterRole/Binding names must include $ns"; exit 1;
  }
done
grep -A8 'kind: ClusterRole$' <<<"$OUT_A" |
  grep -Fq 'app.kubernetes.io/instance: same' || {
  echo "FAIL: ClusterRole must carry the chart labels"; exit 1;
}
echo "PASS: rbac modes"

echo "=== watch.secrets ==="
grep -Fq '"secrets"' <<<"$OUT_LP" || {
  echo "FAIL: Secrets are watched by default"; exit 1;
}
for mode in full least-privilege; do
  OUT_NS=$(helm template test-ns . --set rbac.mode="$mode" \
    --set watch.secrets=false 2>&1)
  if grep -Fq '"secrets"' <<<"$OUT_NS" ||
    grep -Fq 'resources: ["*"]' <<<"$OUT_NS"; then
    echo "FAIL: watch.secrets=false still grants Secrets ($mode)"; exit 1;
  fi
  grep -Fq 'name: KWATCH_WATCH_SECRETS' <<<"$OUT_NS" || {
    echo "FAIL: watch.secrets=false is not passed to kwatch"; exit 1;
  }
done
if grep -Fq 'name: KWATCH_WATCH_SECRETS' <<<"$OUT1"; then
  echo "FAIL: Secrets must be watched by default"; exit 1;
fi
echo "PASS: watch.secrets"

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
