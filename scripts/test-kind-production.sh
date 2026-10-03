#!/usr/bin/env bash
set -euo pipefail

# This is a disposable-cluster smoke test. It validates Kubernetes behavior
# that fake clients cannot prove: RBAC admission, restart recovery, Lease
# handover to a replacement Pod, upgrade retention of the state volume, health
# probes, and an event burst. Kwatch runs as one replica with the Lease as a
# write lock.
release="${KWATCH_RELEASE:-kwatch-ops}"
namespace="${KWATCH_NAMESPACE:-kwatch-ops}"
# Must match the cluster_name the workflow gives helm/kind-action.
cluster="${KIND_CLUSTER_NAME:-kwatch-ops}"
load_count="${KWATCH_LOAD_COUNT:-50}"
rbac_mode="${KWATCH_RBAC_MODE:-full}"
watch_secrets="${KWATCH_WATCH_SECRETS:-true}"
replicas=1
image="${KWATCH_IMAGE:-kwatch:ci}"
port=18060

cleanup() {
	kill "${port_forward_pid:-}" >/dev/null 2>&1 || true
	helm uninstall "$release" --namespace "$namespace" >/dev/null 2>&1 || true
	kubectl delete namespace "$namespace" --ignore-not-found \
		--wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT

case "$load_count" in
	(*[!0-9]*|'') echo "KWATCH_LOAD_COUNT must be numeric" >&2; exit 2;;
esac
case "$rbac_mode" in
	(full|least-privilege) ;;
	(*) echo "KWATCH_RBAC_MODE must be full or least-privilege" >&2; exit 2;;
esac
case "$watch_secrets" in
	(true|false) ;;
	(*) echo "KWATCH_WATCH_SECRETS must be true or false" >&2; exit 2;;
esac
chart_values=(
	--set image.repository="${image%%:*}"
	--set image.tag="${image##*:}"
	--set image.pullPolicy=Never
	--set config.crd.enabled=true
	--set rbac.mode="$rbac_mode"
	--set watch.secrets="$watch_secrets"
)
wait_for_running_replicas() {
	local expected="$1"
	for _ in $(seq 1 90); do
		local count
		count=$(kubectl get pods --namespace "$namespace" \
			-l "app.kubernetes.io/instance=$release" \
			--field-selector=status.phase=Running --no-headers |
			wc -l | tr -d ' ')
		if [[ "$count" == "$expected" ]]; then
			return 0
		fi
		sleep 2
	done
	echo "expected $expected running Pods" >&2
	kubectl get pods --namespace "$namespace" \
		-l "app.kubernetes.io/instance=$release" >&2 || true
	return 1
}

wait_for_deployment_rollout() {
	local expected="$1"
	for _ in $(seq 1 90); do
		local desired updated current
		desired=$(kubectl get deployment "$release" \
			--namespace "$namespace" -o jsonpath='{.spec.replicas}')
		updated=$(kubectl get deployment "$release" \
			--namespace "$namespace" -o jsonpath='{.status.updatedReplicas}')
		current=$(kubectl get deployment "$release" \
			--namespace "$namespace" -o jsonpath='{.status.replicas}')
		if [[ "$desired" == "$expected" && "$updated" == "$expected" &&
			"$current" == "$expected" ]] &&
			wait_for_running_replicas "$expected"; then
			return 0
		fi
		sleep 2
	done
	echo "Deployment rollout did not complete" >&2
	kubectl get deployment "$release" --namespace "$namespace" -o yaml >&2 || true
	return 1
}

assert_leader_pod_available() {
	local ready
	ready=$(kubectl get pod "$leader_pod" --namespace "$namespace" \
		-o 'jsonpath={range .status.conditions[?(@.type=="Ready")]}{.status}{end}')
	if [[ "$ready" != True ]]; then
		echo "Lease holder $leader_pod is not available" >&2
		return 1
	fi
}

echo "Installing $release in $namespace" \
	"(rbac.mode=$rbac_mode, watch.secrets=$watch_secrets)"
kubectl create namespace "$namespace" --dry-run=client -o yaml |
	kubectl apply -f - >/dev/null

kind load docker-image "$image" --name "$cluster"
helm install "$release" deploy/chart \
	--namespace "$namespace" \
	"${chart_values[@]}" \
	--wait=false

wait_for_deployment_rollout "$replicas"

leader_pod=""
for _ in $(seq 1 60); do
	leader_pod=$(kubectl get lease "${release}-leader" \
		--namespace "$namespace" \
		-o jsonpath='{.spec.holderIdentity}' 2>/dev/null || true)
	if [[ -n "$leader_pod" ]] && kubectl get pod "$leader_pod" \
		--namespace "$namespace" >/dev/null 2>&1; then
		break
	fi
	sleep 2
done
if [[ -z "$leader_pod" ]]; then
	echo "leader Lease was not acquired" >&2
	exit 1
fi
leader_count=$(kubectl get lease "${release}-leader" \
	--namespace "$namespace" -o jsonpath='{.spec.holderIdentity}' | \
	awk 'NF {count++} END {print count+0}')
if [[ "$leader_count" != 1 ]]; then
	echo "expected exactly one Lease holder, got $leader_count" >&2
	exit 1
fi
kubectl wait pod "$leader_pod" --namespace "$namespace" \
	--for=condition=Ready --timeout=180s

assert_leader_pod_available

pdb_count=$(kubectl get pdb --namespace "$namespace" \
	-l "app.kubernetes.io/instance=$release" --no-headers |
	wc -l | tr -d ' ')
if [[ "$pdb_count" != 0 ]]; then
	echo "a single-replica release must not create a PodDisruptionBudget" >&2
	exit 1
fi

pvc_phase=$(kubectl get pvc "${release}-data" --namespace "$namespace" \
	-o jsonpath='{.status.phase}')
if [[ "$pvc_phase" != Bound ]]; then
	echo "state volume ${release}-data is not bound: $pvc_phase" >&2
	exit 1
fi

leader_for_lease() {
	kubectl get lease "${release}-leader" \
		--namespace "$namespace" \
		-o jsonpath='{.spec.holderIdentity}' 2>/dev/null || true
}

# await_lease_holder sets leader_pod to the current Lease holder once that
# holder names an existing Pod.
await_lease_holder() {
	local current=""
	for _ in $(seq 1 90); do
		current=$(leader_for_lease)
		if [[ -n "$current" ]] && kubectl get pod "$current" \
			--namespace "$namespace" >/dev/null 2>&1; then
			leader_pod="$current"
			return 0
		fi
		sleep 2
	done
	echo "Lease ${release}-leader has no live holder" >&2
	exit 1
}

wait_for_leader() {
	local previous="${1:-}"
	local current=""
	for _ in $(seq 1 90); do
		current=$(leader_for_lease)
		if [[ -n "$current" && "$current" != "$previous" ]] && \
			kubectl get pod "$current" --namespace "$namespace" \
			>/dev/null 2>&1; then
			leader_pod="$current"
			return 0
		fi
		sleep 2
	done
	echo "leader did not change after the expected transition" >&2
	exit 1
}

# The Deployment recreates a deleted Pod. The replacement must acquire the
# Lease and become available again.
old_leader="$leader_pod"
echo "Deleting Pod $old_leader to test Lease handover"
kubectl delete pod "$old_leader" --namespace "$namespace" \
	--wait=false >/dev/null
wait_for_leader "$old_leader"
kubectl wait pod "$leader_pod" --namespace "$namespace" \
	--for=condition=Ready --timeout=180s
assert_leader_pod_available

assert_state_volume_bound() {
	local phase
	phase=$(kubectl get pvc "${release}-data" --namespace "$namespace" \
		-o jsonpath='{.status.phase}')
	if [[ "$phase" != Bound ]]; then
		echo "state volume was not retained: $phase" >&2
		exit 1
	fi
}

assert_can() {
	local expected="$1"
	shift
	local result
	# can-i exits 1 for "no", which is a valid answer here.
	result=$(kubectl auth can-i "$@" || true)
	if [[ "$result" != "$expected" ]]; then
		echo "RBAC check failed: expected $expected, got $result: $*" >&2
		exit 1
	fi
}

sa="system:serviceaccount:$namespace:$release"
# get on pods is granted only in kwatch's own namespace (restart evidence);
# other namespaces are list/watch only.
assert_can yes --as="$sa" --namespace "$namespace" get pods
assert_can no --as="$sa" --namespace default get pods
assert_can yes --as="$sa" get nodes
assert_can yes --as="$sa" --namespace "$namespace" \
	update "leases/${release}-leader"
# Lease get/update are limited to kwatch's own Lease.
assert_can no --as="$sa" --namespace "$namespace" \
	update leases/not-kwatch
# Kubelet stats are read from each kubelet directly.
assert_can yes --as="$sa" get nodes --subresource=stats
assert_can yes --as="$sa" get nodes --subresource=metrics
assert_can no --as="$sa" get nodes --subresource=proxy
assert_can no --as="$sa" --namespace "$namespace" delete pods
assert_can no --as="$sa" --namespace "$namespace" \
	update configmaps/not-owned
# watch.secrets=false removes every Secret permission in both modes.
if [[ "$watch_secrets" == true ]]; then
	assert_can yes --as="$sa" --namespace default list secrets
else
	assert_can no --as="$sa" --namespace default list secrets
fi
# Only rbac.mode=full (with Secrets watched) lists arbitrary kinds.
if [[ "$rbac_mode" == full && "$watch_secrets" == true ]]; then
	assert_can yes --as="$sa" list widgets.example.com
else
	assert_can no --as="$sa" list widgets.example.com
fi

port_forward_log=$(mktemp)

start_port_forward() {
	local pod="${1:-$leader_pod}"
	kill "${port_forward_pid:-}" >/dev/null 2>&1 || true
	kubectl port-forward --namespace "$namespace" \
		pod/"$pod" "$port:8060" >"$port_forward_log" 2>&1 &
	port_forward_pid=$!
}

# http_ready answers once whether the path works through the port-forward.
http_ready() {
	for _ in $(seq 1 5); do
		if curl --fail --silent "http://127.0.0.1:$port$1" >/dev/null; then
			return 0
		fi
		sleep 2
	done
	return 1
}

# wait_for_kwatch waits until the current Lease holder answers /healthz and
# /readyz. Right after a restart the Lease can still name the Pod that is
# going away, so the holder is looked up and the port-forward restarted on
# every attempt instead of trusting the first answer.
wait_for_kwatch() {
	local when="$1"
	for _ in $(seq 1 30); do
		await_lease_holder
		kubectl wait pod "$leader_pod" --namespace "$namespace" \
			--for=condition=Ready --timeout=60s >/dev/null 2>&1 || true
		start_port_forward "$leader_pod"
		if http_ready /healthz && http_ready /readyz; then
			return 0
		fi
		sleep 5
	done
	echo "kwatch did not become ready $when" >&2
	cat "$port_forward_log" >&2 || true
	exit 1
}

wait_http() {
	local path="$1"
	for _ in $(seq 1 60); do
		if curl --fail --silent "http://127.0.0.1:$port$path" >/dev/null;
		then return 0; fi
		sleep 2
	done
	echo "health endpoint did not become available: $path" >&2
	cat "$port_forward_log" >&2 || true
	exit 1
}

start_port_forward "$leader_pod"
wait_http /healthz
wait_http /readyz
# kwatch_delivery_queue_depth is exported even with no provider configured
# (provider="none"), so its absence means the metrics endpoint is broken.
metrics=$(curl --fail --silent "http://127.0.0.1:$port/metrics")
if ! grep -Eq '^kwatch_delivery_queue_depth([ {]|$)' <<<"$metrics"; then
	echo "metrics endpoint does not export kwatch_delivery_queue_depth" >&2
	exit 1
fi

echo "Generating $load_count pod events"
{
	for i in $(seq 1 "$load_count"); do
		cat <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: kwatch-burst-$i
  namespace: $namespace
spec:
  containers:
  - name: sleeper
    image: registry.k8s.io/pause:3.10
EOF
		echo ---
	done
} | kubectl apply -f - >/dev/null

curl --fail --silent "http://127.0.0.1:$port/healthz" >/dev/null
curl --fail --silent "http://127.0.0.1:$port/readyz" >/dev/null

echo "Testing restart and upgrade retention"
kubectl rollout restart deployment/"$release" --namespace "$namespace"
wait_for_deployment_rollout "$replicas"
wait_for_kwatch "after the restart"
assert_state_volume_bound

helm upgrade "$release" deploy/chart \
	--namespace "$namespace" \
	"${chart_values[@]}" \
	--set podAnnotations.operational-test=upgraded \
	--wait=false
wait_for_deployment_rollout "$replicas"
wait_for_kwatch "after the restart"
assert_state_volume_bound

echo "Testing rollback retention"
helm rollback "$release" 1 --namespace "$namespace" --wait=false
wait_for_deployment_rollout "$replicas"
wait_for_kwatch "after the rollback"
assert_state_volume_bound

# A disposable Kind control-plane restart is a coarse outage/recovery check.
# It validates release recovery after the API/node returns; a true API-only
# fault injection belongs in an operational cluster test.
if [[ "${KWATCH_NODE_RECOVERY:-true}" == true ]]; then
	node=$(kind get nodes --name "$cluster" | awk '/control-plane/ {print $1; exit}')
	test -n "$node"
	echo "Restarting disposable control-plane node $node"
	docker stop "$node" >/dev/null
	docker start "$node" >/dev/null
	# Right after a restart the API server answers before its RBAC rules are
	# loaded, so even the admin user is briefly forbidden to list nodes.
	for _ in $(seq 1 60); do
		if kubectl wait node --all --for=condition=Ready \
			--timeout=5s >/dev/null 2>&1; then
			break
		fi
		sleep 3
	done
	kubectl wait node --all --for=condition=Ready --timeout=60s
	wait_for_deployment_rollout "$replicas"
	wait_for_kwatch "after the node restart"
fi

echo "Kind production smoke test passed."
