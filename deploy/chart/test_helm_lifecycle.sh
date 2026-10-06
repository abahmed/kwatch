#!/usr/bin/env bash
set -euo pipefail

# Requires a disposable Kubernetes cluster, kubectl, and Helm. This verifies
# the behavior that helm template cannot: CRD upgrade and uninstall retention.
release="kwatch-lifecycle"
namespace="kwatch-lifecycle"
second_release="kwatch-lifecycle-b"
second_namespace="kwatch-lifecycle-b"
tmp_chart="$(mktemp -d)"

# Refuse to run against anything but a kind cluster, before anything is
# created and before the EXIT trap can delete things.
root_dir="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck source=scripts/require-kind-context.sh
. "$root_dir/scripts/require-kind-context.sh"
# No argument: any kind-* context is accepted.
require_kind_context "" || exit 2

cleanup() {
  helm uninstall "$release" --namespace "$namespace" >/dev/null 2>&1 || true
  helm uninstall "$second_release" --namespace "$second_namespace" \
    >/dev/null 2>&1 || true
  kubectl delete namespace "$namespace" --ignore-not-found >/dev/null 2>&1 || true
  kubectl delete namespace "$second_namespace" --ignore-not-found \
    >/dev/null 2>&1 || true
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
  # Any field the CRD schema defines; unknown fields would be pruned.
  reasons: ["CrashLoopBackOff"]
EOF

cp -R deploy/chart "$tmp_chart/chart"
# Portable edit (BSD and GNU sed differ on -i): write a new file, then move.
crd_template="$tmp_chart/chart/templates/kwatchconfig-crd.yaml"
awk '{ print } /helm.sh\/resource-policy: keep/ { print "    lifecycle-test: upgraded" }' \
  "$crd_template" >"$crd_template.new"
mv "$crd_template.new" "$crd_template"

helm upgrade "$release" "$tmp_chart/chart" --namespace "$namespace" --wait=false

test "$(kubectl get crd kwatchconfigs.kwatch.abahmed.dev -o jsonpath='{.metadata.annotations.lifecycle-test}')" = upgraded
kubectl get kwatchconfig lifecycle-check --namespace "$namespace" >/dev/null

# A second release elsewhere must install while the first owns the shared
# CRD, and must leave it alone (crd.install lookup guard).
helm install "$second_release" deploy/chart \
  --namespace "$second_namespace" \
  --create-namespace \
  --set image.tag=ci \
  --wait=false
test "$(kubectl get crd kwatchconfigs.kwatch.abahmed.dev \
  -o jsonpath='{.metadata.annotations.meta\.helm\.sh/release-name}')" = "$release"
helm uninstall "$second_release" --namespace "$second_namespace" >/dev/null

helm uninstall "$release" --namespace "$namespace" >/dev/null
kubectl get crd kwatchconfigs.kwatch.abahmed.dev >/dev/null
kubectl get kwatchconfig lifecycle-check --namespace "$namespace" >/dev/null

echo "Helm CRD lifecycle test passed."
