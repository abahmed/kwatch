#!/usr/bin/env bash
set -euo pipefail

# Requires a disposable Kubernetes cluster, kubectl, and Helm. This verifies
# the behavior that helm template cannot: CRD upgrade and uninstall retention.
release="kwatch-lifecycle"
namespace="kwatch-lifecycle"
tmp_chart="$(mktemp -d)"

cleanup() {
  helm uninstall "$release" --namespace "$namespace" >/dev/null 2>&1 || true
  kubectl delete namespace "$namespace" --ignore-not-found >/dev/null 2>&1 || true
  rm -rf "$tmp_chart"
}
trap cleanup EXIT

helm install "$release" deploy/chart \
  --namespace "$namespace" \
  --create-namespace \
  --set image.tag=ci \
  --wait=false

kubectl apply -f - <<'EOF'
apiVersion: kwatch.abahmed.dev/v1alpha1
kind: KwatchConfig
metadata:
  name: lifecycle-check
  namespace: kwatch-lifecycle
spec:
  includeEvents: true
EOF

cp -R deploy/chart "$tmp_chart/chart"
sed -i '/helm.sh\/resource-policy: keep/a\    lifecycle-test: upgraded' \
  "$tmp_chart/chart/templates/kwatchconfig-crd.yaml"

helm upgrade "$release" "$tmp_chart/chart" --namespace "$namespace" --wait=false

test "$(kubectl get crd kwatchconfigs.kwatch.abahmed.dev -o jsonpath='{.metadata.annotations.lifecycle-test}')" = upgraded
kubectl get kwatchconfig lifecycle-check --namespace "$namespace" >/dev/null

helm uninstall "$release" --namespace "$namespace" >/dev/null
kubectl get crd kwatchconfigs.kwatch.abahmed.dev >/dev/null
kubectl get kwatchconfig lifecycle-check --namespace "$namespace" >/dev/null

echo "Helm CRD lifecycle test passed."
