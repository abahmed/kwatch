#!/usr/bin/env bash
set -euo pipefail

# Requires a disposable kind cluster with the current image loaded as
# kwatch:ci. Upgrades a real v0.10.5 install to the current build: Helm
# with the 0.10.5 values file, and the raw manifest, whose Deployment
# selector changed and so needs the Deployment deleted first.
old_tag="v0.10.5"
release="kwatch"
namespace="kwatch-upgrade"
manifest_ns="kwatch"
root_dir="$(cd "$(dirname "$0")/../.." && pwd)"
old_values="$root_dir/deploy/chart/testdata/values-0.10.5.yaml"
old_config="$root_dir/deploy/chart/testdata/values-0.10.5-config.yaml"
work="$(mktemp -d)"

# shellcheck source=scripts/require-kind-context.sh
. "$root_dir/scripts/require-kind-context.sh"
require_kind_context "" || exit 2

cleanup() {
  helm uninstall "$release" --namespace "$namespace" >/dev/null 2>&1 || true
  kubectl delete namespace "$namespace" "$manifest_ns" \
    --ignore-not-found >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

dump() {
  kubectl get pods -A -o wide || true
  kubectl -n "$1" describe deployment || true
  kubectl -n "$1" logs deployment/kwatch --tail=100 || true
}

git -C "$root_dir" fetch --quiet --depth 1 origin \
  "refs/tags/$old_tag:refs/tags/$old_tag"
git -C "$root_dir" archive "$old_tag" deploy | tar -x -C "$work"

echo "=== Helm: $old_tag -> current ==="
# 0.10.x releases were installed with Helm 3 (client-side apply); the
# upgrade keeps the release's method ("auto").
helm install "$release" "$work/deploy/chart" --namespace "$namespace" \
  --server-side=false \
  --create-namespace -f "$old_values" -f "$old_config" --set image.tag="$old_tag" --wait \
  --timeout 3m || { dump "$namespace"; exit 1; }
helm upgrade "$release" "$root_dir/deploy/chart" --namespace "$namespace" \
  -f "$old_values" -f "$old_config" --set image.repository=kwatch --set image.tag=ci \
  --set image.pullPolicy=Never --wait --timeout 5m ||
  { dump "$namespace"; exit 1; }
kubectl -n "$namespace" rollout status deployment/kwatch --timeout=3m ||
  { dump "$namespace"; exit 1; }
image=$(kubectl -n "$namespace" get deployment kwatch \
  -o jsonpath='{.spec.template.spec.containers[0].image}')
test "$image" = "kwatch:ci" || {
  echo "FAIL: upgraded Deployment runs $image"; exit 1;
}
echo "PASS: Helm upgrade"
# Cluster-scoped names (ClusterRole kwatch) are shared with the manifest.
helm uninstall "$release" --namespace "$namespace" --wait >/dev/null

echo "=== Manifest: $old_tag -> current ==="
kubectl apply -f "$work/deploy/deploy.yaml" >/dev/null
if out=$(kubectl apply -f "$root_dir/deploy/deploy.yaml" 2>&1); then
  echo "FAIL: apply over $old_tag was expected to hit the selector change"
  exit 1
fi
grep -Fq "field is immutable" <<<"$out" || {
  echo "FAIL: unexpected apply error: $out"; exit 1;
}
# The documented upgrade steps: the configuration moves from the
# ConfigMap to the Secret kwatch, then delete the Deployment and apply.
kubectl -n "$manifest_ns" create secret generic kwatch \
  --from-literal=config.yaml='app: {clusterName: upgrade-test}'
kubectl -n "$manifest_ns" delete deployment kwatch --wait
kubectl apply -f "$root_dir/deploy/deploy.yaml" >/dev/null
container='/spec/template/spec/containers/0'
kubectl -n "$manifest_ns" patch deployment kwatch --type=json -p "[
  {\"op\":\"replace\",\"path\":\"$container/image\",\"value\":\"kwatch:ci\"},
  {\"op\":\"replace\",\"path\":\"$container/imagePullPolicy\",
   \"value\":\"Never\"}]"
kubectl -n "$manifest_ns" rollout status deployment/kwatch --timeout=3m ||
  { dump "$manifest_ns"; exit 1; }
echo "PASS: manifest upgrade"

echo "Upgrade from $old_tag passed."
