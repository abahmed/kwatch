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
# Karpenter's do-not-disrupt pins the node for as long as the pod runs
# (staging showed hundreds of DisruptionBlocked events), so it is opt-in.
if grep -Fq 'karpenter.sh/do-not-disrupt' <<<"$OUT1"; then
  echo "FAIL: do-not-disrupt must not be a default"; exit 1;
fi
OUT_DND=$(helm template test1 . \
  --set-string 'podAnnotations.karpenter\.sh/do-not-disrupt=true' 2>&1)
grep -Fq 'karpenter.sh/do-not-disrupt: "true"' <<<"$OUT_DND" || {
  echo "FAIL: do-not-disrupt must be settable through podAnnotations"; exit 1;
}
grep -Fq 'cluster-autoscaler.kubernetes.io/safe-to-evict: "false"' \
  <<<"$OUT1" || {
  echo "FAIL: pod must ask the cluster autoscaler not to evict it"; exit 1;
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
  "config.kubelet.bogus=1" "config.watch.bogus=1" \
  "config.resyncSeconds=5" "config.resyncSeconds=-3" \
  "config.app.logFormatter=xml" "config.heartbeatMonitor.interval=-1" \
  "config.activeProbeMonitor.intervalSeconds=0" \
  "config.activeProbeMonitor.bogus=1" \
  "config.activeProbeMonitor.http[0].expectedStatus=99" \
  "networkPolicy.apiServerPorts={}" "crd.bogus=1"; do
  if helm template test-schema . --set "$bad" >/dev/null 2>&1; then
    echo "FAIL: values schema accepted $bad"; exit 1;
  fi
done
helm template test-schema . --set config.kubelet.insecureSkipVerify=true \
  --set config.watch.secrets=true >/dev/null || {
  echo "FAIL: values schema rejects config.kubelet or config.watch"; exit 1;
}
for good in "config.resyncSeconds=0" "config.resyncSeconds=30" \
  "config.app.logFormatter=json" "config.app.caBundlePath=/etc/ca.pem" \
  "config.app.insecureSkipTLSVerify=false" "config.heartbeatMonitor.interval=0" \
  "config.activeProbeMonitor.autoServices=true" "crd.install=false"; do
  helm template test-schema . --set "$good" >/dev/null || {
    echo "FAIL: values schema rejects $good"; exit 1;
  }
done
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

echo "=== resource defaults ==="
grep -Fq "cpu: 500m" <<<"$OUT1" || {
  echo "FAIL: cpu limit must be 500m"; exit 1;
}
echo "PASS: resource defaults"

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
# The health port is not open to every pod: only the release namespace and
# the peers in healthIngressFrom.
INGRESS_DEFAULT=$(sed -n '/^  ingress:/,/^  egress:/p' <<<"$OUT2")
grep -Fq "kubernetes.io/metadata.name: default" <<<"$INGRESS_DEFAULT" || {
  echo "FAIL: health ingress is not limited to the release namespace"; exit 1;
}
OUT_NP_FROM=$(helm template test-np . --set networkPolicy.enabled=true \
  --set 'networkPolicy.healthIngressFrom[0].podSelector.matchLabels.app=prom' \
  2>&1)
grep -Fq "app: prom" <<<"$OUT_NP_FROM" || {
  echo "FAIL: network policy ignores healthIngressFrom"; exit 1;
}
grep -Fq "port: 9153" <<<"$OUT2" || {
  echo "FAIL: network policy must open the DNS metrics port"; exit 1;
}
OUT_NP_NODNS=$(helm template test-np . --set networkPolicy.enabled=true \
  --set networkPolicy.dnsMetricsPort=0 2>&1)
if grep -Fq "port: 9153" <<<"$OUT_NP_NODNS"; then
  echo "FAIL: dnsMetricsPort=0 must omit the DNS metrics rule"; exit 1;
fi
if grep -Fq -- "- {}" <<<"$OUT2"; then
  echo "FAIL: all-egress rule must not be rendered by default"; exit 1;
fi
OUT_NP_ALL=$(helm template test-np . --set networkPolicy.enabled=true \
  --set networkPolicy.allowProbeEgressAll=true 2>&1)
grep -Fq -- "- {}" <<<"$OUT_NP_ALL" || {
  echo "FAIL: allowProbeEgressAll must render an all-egress rule"; exit 1;
}
echo "PASS: optional network policy"

echo "=== rbac modes ==="
if grep -Fq 'resources: ["*"]' <<<"$OUT1"; then
  echo "FAIL: default rbac.mode must be least-privilege"; exit 1;
fi
OUT_FULL=$(helm template test-full . --set rbac.mode=full 2>&1)
grep -Fq 'resources: ["*"]' <<<"$OUT_FULL" || {
  echo "FAIL: rbac.mode=full must use wildcard list access"; exit 1;
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
for out in "$OUT1" "$OUT_FULL" "$OUT_LP"; do
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

echo "=== KwatchConfig RBAC is namespaced ==="
# The overlay is read from kwatch's own namespace, so list/watch on
# kwatchconfigs is a Role, never a ClusterRole rule, and only with the CRD
# overlay enabled.
if sed -n '/^kind: ClusterRole$/,/^---$/p' <<<"$OUT_LP" |
  grep -Fq kwatchconfigs; then
  echo "FAIL: ClusterRole must not grant kwatchconfigs"; exit 1;
fi
grep -Fq "name: test-lp-config-overlay" <<<"$OUT_LP" || {
  echo "FAIL: namespaced kwatchconfigs Role is missing"; exit 1;
}
OUT_NOCRD=$(helm template test-nocrd . --set rbac.mode=least-privilege \
  --set config.crd.enabled=false 2>&1)
if grep -Fq "config-overlay" <<<"$OUT_NOCRD"; then
  echo "FAIL: no kwatchconfigs access without the CRD overlay"; exit 1;
fi
echo "PASS: KwatchConfig RBAC"

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

# The chart template is deploy/crd.yaml wrapped in the crd.install guard.
sed '1d;$d' templates/kwatchconfig-crd.yaml | cmp -s ../crd.yaml - || {
  echo "FAIL: chart CRD is out of sync with deploy/crd.yaml"
  exit 1
}
echo "PASS: CRD is in sync"

grep -Fq "kind: CustomResourceDefinition" <<<"$OUT1" || {
  echo "FAIL: CRD is not rendered by the chart"
  exit 1
}
OUT_NOINSTALL=$(helm template test-crd . --set crd.install=false 2>&1)
if grep -Fq "kind: CustomResourceDefinition" <<<"$OUT_NOINSTALL"; then
  echo "FAIL: crd.install=false must not render the CRD"
  exit 1
fi
grep -Fq "name: test-crd-config-overlay" <<<"$OUT_NOINSTALL" || {
  echo "FAIL: crd.install=false must keep the overlay RBAC"
  exit 1
}
echo "PASS: CRD is rendered"

echo "=== CRD validation matches the binary ==="
grep -Fq 'self == 0 || self >= 30' ../crd.yaml || {
  echo "FAIL: CRD must validate resyncSeconds (0 or >= 30)"; exit 1;
}
grep -Fq -- '- json' ../crd.yaml || {
  echo "FAIL: CRD must restrict app.logFormatter"; exit 1;
}
# A KwatchConfig must not disable or move the health server.
if grep -Fq 'healthCheck:' ../crd.yaml; then
  echo "FAIL: CRD must not offer healthCheck"; exit 1;
fi
echo "PASS: CRD validation"

echo "=== Secret access warning in NOTES.txt ==="
# helm template never prints NOTES, so render a copy as a manifest.
NOTES_DIR=$(mktemp -d)
trap 'rm -rf "$NOTES_DIR"' EXIT
cp -R . "$NOTES_DIR/chart"
{ printf 'notes: |\n  #\n'; sed 's/^/  /' templates/NOTES.txt; } \
  >"$NOTES_DIR/chart/templates/notes-test.yaml"
render_notes() {
  helm template test-notes "$NOTES_DIR/chart" \
    --show-only templates/notes-test.yaml "$@" 2>&1
}
OUT_NOTES=$(render_notes)
for want in "can read" "every Secret" "watch.secrets=false"; do
  if ! grep -Fq "$want" <<<"$OUT_NOTES"; then
    echo "FAIL: default install must warn that Secrets are readable"
    exit 1
  fi
done
grep -Fq "rbac.mode=least-privilege" <<<"$OUT_NOTES" || {
  echo "FAIL: warning must say how to restrict"; exit 1;
}
if grep -Fq "every Secret" <<<"$(render_notes --set watch.secrets=false)"; then
  echo "FAIL: watch.secrets=false must not warn about Secret access"; exit 1;
fi
# Least-privilege hides custom resources; say so, and say when rbac.mode=full
# is overridden by watch.secrets=false.
for want in "Custom" "not watched" "permission_denied" "rbac.mode=full"; do
  grep -Fq "$want" <<<"$OUT_NOTES" || {
    echo "FAIL: the default install must say custom resources are not watched ($want)"; exit 1;
  }
done
if grep -Fq "permission_denied" <<<"$(render_notes --set rbac.mode=full)"; then
  echo "FAIL: rbac.mode=full must not say custom resources are hidden"; exit 1;
fi
OUT_FULL_NS=$(render_notes --set rbac.mode=full --set watch.secrets=false)
grep -Fq "rbac.mode=full is not in effect" <<<"$OUT_FULL_NS" || {
  echo "FAIL: full with watch.secrets=false must say it is least-privilege"; exit 1;
}
# Providers need a Secret: without configSecretName none is configured.
grep -Fq "no alert provider is configured" <<<"$OUT_NOTES" || {
  echo "FAIL: an install with no providers must say so"; exit 1;
}
if grep -Fq "no alert provider" <<<"$(render_notes --set configSecretName=kwatch-config)"; then
  echo "FAIL: configSecretName holds the providers; do not warn"; exit 1;
fi
echo "PASS: Secret access warning"

echo "=== license ==="
# No key: the env var reads an optional Secret and nothing is created.
grep -A6 -F "name: KWATCH_LICENSE_KEY" <<<"$OUT1" |
  grep -Fq "optional: true" || {
  echo "FAIL: KWATCH_LICENSE_KEY must be an optional secretKeyRef"; exit 1;
}
grep -A4 -F "name: KWATCH_LICENSE_KEY" <<<"$OUT1" |
  grep -Fq "name: test1-license" || {
  echo "FAIL: the default license Secret is <release>-license"; exit 1;
}
if helm template test1 . --show-only templates/license-secret.yaml \
  >/dev/null 2>&1; then
  echo "FAIL: no Secret may be created without license.key"; exit 1;
fi
OUT_LIC=$(helm template test-lic . --set license.key=KW1-test 2>&1)
grep -Fq 'license-key: "KW1-test"' <<<"$OUT_LIC" || {
  echo "FAIL: license.key must create <release>-license"; exit 1;
}
OUT_LIC_EX=$(helm template test-lic . --set license.key=KW1-test \
  --set license.existingSecret=my-license 2>&1)
grep -A4 -F "name: KWATCH_LICENSE_KEY" <<<"$OUT_LIC_EX" |
  grep -Fq "name: my-license" || {
  echo "FAIL: license.existingSecret must be referenced"; exit 1;
}
if grep -Fq 'license-key: "KW1-test"' <<<"$OUT_LIC_EX"; then
  echo "FAIL: existingSecret wins over license.key"; exit 1;
fi
# One get/update rule on a single name and no create, in every rbac.mode
# and with Secret watching on or off.
for mode in full least-privilege; do
  for secrets in true false; do
    OUT_LR=$(helm template test-lic . --set license.key=KW1-test \
      --set rbac.mode="$mode" \
      --set watch.secrets="$secrets" --show-only templates/rbac.yaml 2>&1)
    ROLE=$(awk -v RS='\n---\n' '/kind: Role\n/ && /name: test-lic-license\n/' \
      <<<"$OUT_LR")
    [ -n "$ROLE" ] || {
      echo "FAIL: license Role missing ($mode, secrets=$secrets)"; exit 1;
    }
    if ! { [ "$(grep -c 'resourceNames:' <<<"$ROLE")" = 1 ] &&
      [ "$(grep -c -- '- kwatch-license-instance' <<<"$ROLE")" = 1 ] &&
      [ "$(grep -c -- '- get' <<<"$ROLE")" = 1 ] &&
      [ "$(grep -c -- '- update' <<<"$ROLE")" = 1 ] &&
      ! grep -Eq -- '- (create|delete|list|watch|patch|"\*")' <<<"$ROLE"; }; then
      echo "FAIL: license Role must be get/update on one name only ($mode, secrets=$secrets)"; exit 1
    fi
  done
done
# Without a key the chart renders neither the instance Secret nor the Role
# that reads it; with a key (or license.instanceSecret) it ships the Secret
# empty, kept on uninstall and without data, so Helm's three-way merge
# leaves kwatch's key alone.
if helm template test-lic . --show-only templates/license-instance-secret.yaml \
  >/dev/null 2>&1; then
  echo "FAIL: no instance Secret may be rendered without a license key"; exit 1
fi
if grep -Fq "name: test1-license" <<<"$(helm template test1 . \
  --show-only templates/rbac.yaml 2>&1)"; then
  echo "FAIL: no license Role may be rendered without a license key"; exit 1
fi
for set_args in "--set license.key=KW1-test" \
  "--set license.existingSecret=my-license" \
  "--set license.instanceSecret=true"; do
  # shellcheck disable=SC2086
  OUT_IS=$(helm template test-lic . $set_args \
    --show-only templates/license-instance-secret.yaml 2>&1)
  if ! { grep -Fq "name: kwatch-license-instance" <<<"$OUT_IS" &&
    grep -Fq "type: Opaque" <<<"$OUT_IS" &&
    grep -Fq "helm.sh/resource-policy: keep" <<<"$OUT_IS" &&
    ! grep -Eq '^(data|stringData):' <<<"$OUT_IS"; }; then
    echo "FAIL: instance Secret must be empty, Opaque and kept ($set_args)"; exit 1
  fi
  # shellcheck disable=SC2086
  OUT_ROLE=$(helm template test-lic . $set_args --show-only templates/rbac.yaml 2>&1)
  grep -Fq "name: test-lic-license" <<<"$OUT_ROLE" || {
    echo "FAIL: license Role missing ($set_args)"; exit 1;
  }
done
# An existing license Secret rolls the pod when its content changes; an
# offline render hashes the name, and the key is read once at start.
OUT_ROLL=$(helm template test-lic . --set license.existingSecret=my-license 2>&1)
grep -Fq "checksum/license-secret:" <<<"$OUT_ROLL" || {
  echo "FAIL: existingSecret must set a checksum annotation"; exit 1;
}
# HTTPS egress for the license only with a license configured.
OUT_NNP=$(helm template test-lnp . --set networkPolicy.enabled=true \
  --set 'networkPolicy.apiServerPorts={6443}' 2>&1)
if grep -Fq "License activation" <<<"$OUT_NNP"; then
  echo "FAIL: networkPolicy must not open HTTPS egress without a license"; exit 1
fi
for set_args in "--set license.key=KW1-test" \
  "--set license.existingSecret=my-license" \
  "--set license.instanceSecret=true"; do
  # shellcheck disable=SC2086
  OUT_LNP=$(helm template test-lnp . --set networkPolicy.enabled=true \
    --set 'networkPolicy.apiServerPorts={6443}' $set_args 2>&1)
  if ! grep -A7 -F "License activation" <<<"$OUT_LNP" |
    grep -Fq "port: 443"; then
    echo "FAIL: networkPolicy must allow HTTPS egress for the license ($set_args)"; exit 1
  fi
done
if helm template test-lic . --set license.bogus=1 >/dev/null 2>&1; then
  echo "FAIL: license.bogus must be rejected by the schema"; exit 1;
fi
echo "PASS: license"

echo "=== pod scheduling and volumes ==="
OUT_POD=$(helm template test-pod . --set priorityClassName=kwatch-critical \
  --set 'imagePullSecrets[0].name=regcred' \
  --set 'extraVolumes[0].name=ca' --set 'extraVolumes[0].secret.secretName=ca' \
  --set 'extraVolumeMounts[0].name=ca' \
  --set 'extraVolumeMounts[0].mountPath=/etc/ca' 2>&1)
grep -Fq 'priorityClassName: "kwatch-critical"' <<<"$OUT_POD" || {
  echo "FAIL: priorityClassName must reach the pod"; exit 1;
}
grep -A1 -F "imagePullSecrets:" <<<"$OUT_POD" | grep -Fq "name: regcred" || {
  echo "FAIL: imagePullSecrets must reach the pod"; exit 1;
}
for want in "mountPath: /etc/ca" "secretName: ca"; do
  if ! grep -Fq "$want" <<<"$OUT_POD"; then
    echo "FAIL: extra volumes and mounts must reach the pod"; exit 1
  fi
done
if grep -Fq "priorityClassName" <<<"$OUT1" ||
  grep -Fq "imagePullSecrets" <<<"$OUT1"; then
  echo "FAIL: scheduling extras must be absent by default"; exit 1
fi
if helm template test-pod . --set 'extraVolumes[0].name=data' \
  --set 'extraVolumes[0].emptyDir={}' >/dev/null 2>&1; then
  echo "FAIL: extraVolumes must not reuse a chart volume name"; exit 1
fi
echo "PASS: pod scheduling and volumes"

echo "=== 0.10.5 values file ==="
# A values file from the 0.10.5 chart (verbatim, with its empty config:
# and the removed service: block) must still render for helm upgrade.
OUT_OLD=$(helm template test-old . -f testdata/values-0.10.5.yaml 2>&1) || {
  echo "FAIL: 0.10.5 values file must render: $OUT_OLD"; exit 1;
}
if grep -Fq "port: 8060" <<<"$OUT_OLD"; then
  echo "FAIL: the deprecated service block must be ignored"; exit 1;
fi
if helm template test-old . --set bogusTopLevel=1 >/dev/null 2>&1; then
  echo "FAIL: unknown top-level keys must still be rejected"; exit 1;
fi
echo "PASS: 0.10.5 values file"

echo "=== dashboard ==="
for kind in "kind: Ingress" "KWATCH_DASHBOARD_ENABLED" "port: 8061"; do
  if grep -Fq "$kind" <<<"$OUT1"; then
    echo "FAIL: the dashboard must be off by default ($kind)"; exit 1
  fi
done
OUT_DASH=$(helm template kwatch . --set dashboard.enabled=true 2>&1)
for want in 'name: KWATCH_DASHBOARD_PASSWORD' 'key: password' \
  'name: KWATCH_DASHBOARD_MODE' 'value: "local"' \
  'name: KWATCH_DEPLOYMENT_NAME' '  password: ' '  token: '; do
  grep -Fq -- "$want" <<<"$OUT_DASH" || {
    echo "FAIL: dashboard render lacks $want"; exit 1;
  }
done
if grep -Fq "kind: Ingress" <<<"$OUT_DASH"; then
  echo "FAIL: local mode must not render an Ingress"; exit 1
fi
PW=$(grep -E '^  password: ' <<<"$OUT_DASH" | awk '{print $2}' |
  base64 --decode)
grep -Eq '^([A-HJ-NP-Z2-9]{4}-){3}[A-HJ-NP-Z2-9]{4}$' <<<"$PW" || {
  echo "FAIL: generated password has the wrong shape: $PW"; exit 1;
}
# expose_ok renders with dashboard.enabled and the given flags and checks
# every following string is present; expose_fail checks the render fails
# and names the given text.
expose_ok() {
  local label=$1; shift
  local args=()
  while [ "$1" != "--" ]; do args+=("$1"); shift; done; shift
  local out
  out=$(helm template kwatch . --set dashboard.enabled=true "${args[@]}" \
    2>&1) || { echo "FAIL: $label must render: $out"; exit 1; }
  for want in "$@"; do
    grep -Fq -- "$want" <<<"$out" || {
      echo "FAIL: $label lacks $want"; exit 1;
    }
  done
}
expose_fail() {
  local label=$1 want=$2; shift 2
  local out
  if out=$(helm template kwatch . --set dashboard.enabled=true "$@" 2>&1)
  then
    echo "FAIL: $label must not render"; exit 1
  fi
  grep -Fq -- "$want" <<<"$out" || {
    echo "FAIL: $label: message lacks '$want': $out"; exit 1;
  }
}
H=(--set dashboard.host=kwatch.example.com)
expose_fail "unknown expose" "must be one of" --set dashboard.expose=public
expose_ok "service" --set dashboard.expose=service -- \
  "type: ClusterIP" 'value: "local"'
expose_fail "ingress without host" "needs dashboard.host" \
  --set dashboard.expose=ingress
expose_fail "ingress without TLS" "needs TLS" \
  --set dashboard.expose=ingress "${H[@]}"
expose_ok "ingress nginx" --set dashboard.expose=ingress "${H[@]}" \
  --set dashboard.ingress.clusterIssuer=letsencrypt-prod \
  --set dashboard.ingress.className=nginx \
  --set 'dashboard.allowedSourceRanges={10.0.0.0/8}' -- \
  "kind: Ingress" 'cert-manager.io/cluster-issuer: "letsencrypt-prod"' \
  "secretName: kwatch-dashboard-tls" "ingressClassName: nginx" \
  'whitelist-source-range: "10.0.0.0/8"' 'value: "shared"' \
  'name: KWATCH_DASHBOARD_TRUST_PROXY'
expose_fail "alb without certificate" "certificateArn" \
  --set dashboard.expose=ingress "${H[@]}" \
  --set dashboard.ingress.provider=alb
expose_ok "ingress alb" --set dashboard.expose=ingress "${H[@]}" \
  --set dashboard.ingress.provider=alb \
  --set dashboard.ingress.alb.scheme=internal \
  --set dashboard.ingress.alb.certificateArn=arn:aws:acm:x \
  --set 'dashboard.allowedSourceRanges={10.0.0.0/8}' -- \
  'alb.ingress.kubernetes.io/scheme: "internal"' \
  "alb.ingress.kubernetes.io/target-type: ip" \
  'alb.ingress.kubernetes.io/certificate-arn: "arn:aws:acm:x"' \
  'inbound-cidrs: "10.0.0.0/8"' "ingressClassName: alb"
expose_ok "ingress gce" --set dashboard.expose=ingress "${H[@]}" \
  --set dashboard.ingress.provider=gce \
  --set dashboard.ingress.gce.managedCertificate=true -- \
  "kind: ManagedCertificate" "ingressClassName: gce" \
  'kubernetes.io/ingress.allow-http: "false"'
expose_ok "ingress agic" --set dashboard.expose=ingress "${H[@]}" \
  --set dashboard.ingress.provider=agic \
  --set dashboard.ingress.tlsSecretName=t -- \
  "ingressClassName: azure-application-gateway" "secretName: t"
expose_ok "ingress tailscale" --set dashboard.expose=ingress \
  --set dashboard.ingress.provider=tailscale -- \
  "ingressClassName: tailscale" "defaultBackend:"
expose_ok "ingress ngrok" --set dashboard.expose=ingress "${H[@]}" \
  --set dashboard.ingress.provider=ngrok -- "ingressClassName: ngrok"
expose_fail "ranges on gce" "not supported" \
  --set dashboard.expose=ingress "${H[@]}" \
  --set dashboard.ingress.provider=gce \
  --set dashboard.ingress.gce.managedCertificate=true \
  --set 'dashboard.allowedSourceRanges={10.0.0.0/8}'
expose_fail "gateway without name" "dashboard.gateway.name" \
  --set dashboard.expose=gateway "${H[@]}"
expose_ok "gateway" --set dashboard.expose=gateway "${H[@]}" \
  --set dashboard.gateway.name=public \
  --set dashboard.gateway.namespace=gw -- \
  "kind: HTTPRoute" "namespace: gw" 'hostnames:'
expose_fail "istio without gateway" "dashboard.istio.gateway" \
  --set dashboard.expose=istio "${H[@]}"
expose_ok "istio" --set dashboard.expose=istio "${H[@]}" \
  --set dashboard.istio.gateway=istio-system/public -- \
  "kind: VirtualService" "istio-system/public"
expose_ok "route" --set dashboard.expose=route \
  --set 'dashboard.allowedSourceRanges={10.0.0.0/8}' -- \
  "kind: Route" "termination: edge" \
  "insecureEdgeTerminationPolicy: Redirect" "ip_whitelist"
expose_fail "loadBalancer without TLS" "exactly one of" \
  --set dashboard.expose=loadBalancer
expose_fail "internal LB without provider" "loadBalancer.provider" \
  --set dashboard.expose=loadBalancer --set dashboard.tls.selfSigned=true \
  --set dashboard.loadBalancer.internal=true
expose_ok "loadBalancer" --set dashboard.expose=loadBalancer \
  --set dashboard.tls.selfSigned=true \
  --set dashboard.loadBalancer.internal=true \
  --set dashboard.loadBalancer.provider=aws \
  --set dashboard.loadBalancer.loadBalancerClass=service.k8s.aws/nlb \
  --set 'dashboard.allowedSourceRanges={10.0.0.0/8}' -- \
  "type: LoadBalancer" "port: 443" "type: kubernetes.io/tls" \
  'aws-load-balancer-scheme: internal' "loadBalancerSourceRanges:" \
  "loadBalancerClass: service.k8s.aws/nlb" \
  "name: KWATCH_DASHBOARD_TLS_CERT" "mountPath: /etc/kwatch/dashboard-tls"
expose_ok "loadBalancer gcp internal" --set dashboard.expose=loadBalancer \
  --set dashboard.tls.secretName=mine \
  --set dashboard.loadBalancer.internal=true \
  --set dashboard.loadBalancer.provider=gcp -- \
  'networking.gke.io/load-balancer-type: Internal' "secretName: mine"
expose_ok "nodePort" --set dashboard.expose=nodePort \
  --set dashboard.tls.issuer=letsencrypt-prod \
  --set dashboard.nodePort.port=30443 -- \
  "type: NodePort" "nodePort: 30443" "kind: Certificate" \
  "name: letsencrypt-prod"
expose_fail "external without trust" "trustProxy=true" \
  --set dashboard.expose=external
expose_ok "external" --set dashboard.expose=external \
  --set dashboard.external.trustProxy=true -- \
  'value: "shared"' 'name: KWATCH_DASHBOARD_TRUST_PROXY'
expose_fail "ranges on external" "not supported" \
  --set dashboard.expose=external --set dashboard.external.trustProxy=true \
  --set 'dashboard.allowedSourceRanges={10.0.0.0/8}'
expose_ok "networkPolicy loadBalancer" --set networkPolicy.enabled=true \
  --set dashboard.expose=loadBalancer --set dashboard.tls.selfSigned=true \
  --set 'dashboard.allowedSourceRanges={10.0.0.0/8}' -- "cidr: 10.0.0.0/8"
OUT_EXIST=$(helm template kwatch . --set dashboard.enabled=true \
  --set dashboard.existingSecret=mine 2>&1)
if grep -Eq '^  password: ' <<<"$OUT_EXIST"; then
  echo "FAIL: existingSecret must not render a password Secret"; exit 1
fi
grep -Fq "name: mine" <<<"$OUT_EXIST" || {
  echo "FAIL: existingSecret must feed the dashboard env"; exit 1;
}
echo "PASS: dashboard"

echo "All helm template tests passed."
