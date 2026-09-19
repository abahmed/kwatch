#!/usr/bin/env bash
set -euo pipefail

# This is a disposable-cluster smoke test. It validates Kubernetes behavior
# that fake clients cannot prove: RBAC admission, ConfigMap migration, restart
# recovery, upgrade retention, health probes, and an event burst.
release="${KWATCH_RELEASE:-kwatch-ops}"
namespace="${KWATCH_NAMESPACE:-kwatch-ops}"
cluster="${KIND_CLUSTER_NAME:-kind}"
load_count="${KWATCH_LOAD_COUNT:-50}"
replicas="${KWATCH_REPLICAS:-2}"
scale_test="${KWATCH_SCALE_TEST:-false}"
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
case "$replicas" in
	(*[!0-9]*|''|0) echo "KWATCH_REPLICAS must be positive" >&2; exit 2;;
esac

wait_for_running_replicas() {
	local expected="$1"
	for attempt in $(seq 1 90); do
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
	for attempt in $(seq 1 90); do
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

echo "Installing $release in $namespace"
kubectl create namespace "$namespace" --dry-run=client -o yaml |
	kubectl apply -f - >/dev/null

# Seed an older state schema before startup. The application must migrate it
# without replacing the ConfigMap or losing the cluster identity.
kubectl create configmap kwatch-state \
	--namespace "$namespace" \
	--from-literal=state-schema-version=1 \
	--from-literal=cluster-id=operational-test \
	--dry-run=client -o yaml | kubectl apply -f - >/dev/null

kind load docker-image "$image" --name "$cluster"
helm install "$release" deploy/chart \
	--namespace "$namespace" \
	--set image.repository="${image%%:*}" \
	--set image.tag="${image##*:}" \
	--set image.pullPolicy=Never \
	--set replicaCount="$replicas" \
	--set config.crd.enabled=true \
	--wait=false

wait_for_deployment_rollout "$replicas"

leader_pod=""
for attempt in $(seq 1 60); do
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

if [[ "$replicas" -gt 1 ]]; then
	pdb_count=$(kubectl get pdb --namespace "$namespace" \
		-l "app.kubernetes.io/instance=$release" --no-headers |
		wc -l | tr -d ' ')
	if [[ "$pdb_count" != 1 ]]; then
		echo "expected one PodDisruptionBudget, got $pdb_count" >&2
		exit 1
	fi
else
	pdb_count=$(kubectl get pdb --namespace "$namespace" \
		-l "app.kubernetes.io/instance=$release" --no-headers |
		wc -l | tr -d ' ')
	if [[ "$pdb_count" != 0 ]];
	then
		echo "a single-replica release must not create a PodDisruptionBudget" >&2
		exit 1
	fi
fi

leader_for_lease() {
	kubectl get lease "${release}-leader" \
		--namespace "$namespace" \
		-o jsonpath='{.spec.holderIdentity}' 2>/dev/null || true
}

wait_for_leader() {
	local previous="${1:-}"
	local current=""
	for attempt in $(seq 1 90); do
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

if [[ "$replicas" -gt 1 ]]; then
	old_leader="$leader_pod"
	echo "Deleting leader $old_leader to test standby takeover"
	kubectl delete pod "$old_leader" --namespace "$namespace" \
		--wait=false >/dev/null
	wait_for_leader "$old_leader"
	kubectl wait pod "$leader_pod" --namespace "$namespace" \
		--for=condition=Ready --timeout=180s
	assert_leader_pod_available
fi

if [[ "$scale_test" == true ]]; then
	echo "Testing scale-up, standby removal, and scale-down"
	helm upgrade "$release" deploy/chart \
		--namespace "$namespace" \
		--set image.repository="${image%%:*}" \
		--set image.tag="${image##*:}" \
		--set image.pullPolicy=Never \
		--set replicaCount=5 \
		--set config.crd.enabled=true \
		--wait=false
	wait_for_running_replicas 5
	leader_before_standby_delete=$(leader_for_lease)
	standby_pod=$(kubectl get pods --namespace "$namespace" \
		-l "app.kubernetes.io/instance=$release" \
		-o custom-columns='NAME:.metadata.name' --no-headers |
		awk -v leader="$leader_before_standby_delete" '$1 != leader {print $1; exit}')
	test -n "$standby_pod"
	kubectl delete pod "$standby_pod" --namespace "$namespace" \
		--wait=false >/dev/null
	for attempt in $(seq 1 90); do
		[[ "$(leader_for_lease)" == "$leader_before_standby_delete" ]] && break
		sleep 2
	done
	if [[ "$(leader_for_lease)" != "$leader_before_standby_delete" ]]; then
		echo "deleting a standby unexpectedly changed the leader" >&2
		exit 1
	fi
	assert_leader_pod_available

	echo "Testing leader removal after scale-up"
	kubectl delete pod "$leader_before_standby_delete" \
		--namespace "$namespace" --wait=false >/dev/null
	wait_for_leader "$leader_before_standby_delete"
	kubectl wait pod "$leader_pod" --namespace "$namespace" \
		--for=condition=Ready --timeout=180s
	assert_leader_pod_available

	helm upgrade "$release" deploy/chart \
		--namespace "$namespace" \
		--set image.repository="${image%%:*}" \
		--set image.tag="${image##*:}" \
		--set image.pullPolicy=Never \
		--set replicaCount=2 \
		--set config.crd.enabled=true \
		--wait=false
	wait_for_running_replicas 2
	leader_pod=$(leader_for_lease)
	kubectl wait pod "$leader_pod" --namespace "$namespace" \
		--for=condition=Ready --timeout=180s
	assert_leader_pod_available
fi

kubectl get configmap kwatch-state --namespace "$namespace" \
	-o jsonpath='{.data.state-schema-version}' | grep -Fx 2
kubectl get configmap kwatch-state --namespace "$namespace" \
	-o jsonpath='{.data.cluster-id}' | grep -Fx operational-test

assert_can() {
	local expected="$1"
	shift
	local result
	result=$(kubectl auth can-i "$@")
	if [[ "$result" != "$expected" ]]; then
		echo "RBAC check failed: expected $expected, got $result: $*" >&2
		exit 1
	fi
}

sa="system:serviceaccount:$namespace:$release"
assert_can yes --as="$sa" --namespace "$namespace" get pods
assert_can yes --as="$sa" --namespace "$namespace" create configmaps
assert_can yes --as="$sa" --namespace "$namespace" \
	update configmaps --resource-name kwatch-state
assert_can no --as="$sa" --namespace "$namespace" delete pods
assert_can no --as="$sa" --namespace "$namespace" \
	update configmaps --resource-name not-owned

port_forward_log=$(mktemp)

start_port_forward() {
	local pod="${1:-$leader_pod}"
	kill "${port_forward_pid:-}" >/dev/null 2>&1 || true
	kubectl port-forward --namespace "$namespace" \
		pod/"$pod" "$port:8060" >"$port_forward_log" 2>&1 &
	port_forward_pid=$!
}

wait_http() {
	local path="$1"
	local attempt
	for attempt in $(seq 1 60); do
		if curl --fail --silent "http://127.0.0.1:$port$path" >/dev/null;
		then return 0; fi
		sleep 2
	done
	echo "health endpoint did not become available: $path" >&2
	cat "$port_forward_log" >&2 || true
	exit 1
}

if [[ "$replicas" -gt 1 ]]; then
	standby_pod=$(kubectl get pods --namespace "$namespace" \
		-l "app.kubernetes.io/instance=$release" \
		-o custom-columns='NAME:.metadata.name' --no-headers |
		awk -v leader="$leader_pod" '$1 != leader {print $1; exit}')
	test -n "$standby_pod"
	start_port_forward "$standby_pod"
	wait_http /healthz
	if curl --fail --silent "http://127.0.0.1:$port/readyz" \
		>/dev/null; then
		echo "standby Pod unexpectedly reported ready" >&2
		exit 1
	fi
fi

start_port_forward "$leader_pod"
wait_http /healthz
wait_http /readyz
curl --fail --silent "http://127.0.0.1:$port/metrics" |
	grep -E '^kwatch_queue_depth([ {]|$)' >/dev/null

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
leader_pod=$(kubectl get lease "${release}-leader" \
	--namespace "$namespace" -o jsonpath='{.spec.holderIdentity}')
kubectl wait pod "$leader_pod" --namespace "$namespace" \
	--for=condition=Ready --timeout=180s
start_port_forward
wait_http /readyz
kubectl get configmap kwatch-state --namespace "$namespace" >/dev/null

helm upgrade "$release" deploy/chart \
	--namespace "$namespace" \
	--set image.repository="${image%%:*}" \
	--set image.tag="${image##*:}" \
	--set image.pullPolicy=Never \
	--set replicaCount="$replicas" \
	--set config.crd.enabled=true \
	--set podAnnotations.operational-test=upgraded \
	--wait=false
wait_for_deployment_rollout "$replicas"
leader_pod=$(kubectl get lease "${release}-leader" \
	--namespace "$namespace" -o jsonpath='{.spec.holderIdentity}')
kubectl wait pod "$leader_pod" --namespace "$namespace" \
	--for=condition=Ready --timeout=180s
start_port_forward
wait_http /readyz
kubectl get configmap kwatch-state --namespace "$namespace" >/dev/null

echo "Testing rollback retention"
helm rollback "$release" 1 --namespace "$namespace" --wait=false
wait_for_deployment_rollout "$replicas"
leader_pod=$(kubectl get lease "${release}-leader" \
	--namespace "$namespace" -o jsonpath='{.spec.holderIdentity}')
kubectl wait pod "$leader_pod" --namespace "$namespace" \
	--for=condition=Ready --timeout=180s
start_port_forward "$leader_pod"
wait_http /readyz
kubectl get configmap kwatch-state --namespace "$namespace" >/dev/null

# A disposable Kind control-plane restart is a coarse outage/recovery check.
# It validates release recovery after the API/node returns; a true API-only
# fault injection belongs in an operational cluster test.
if [[ "${KWATCH_NODE_RECOVERY:-true}" == true ]]; then
	node=$(kind get nodes --name "$cluster" | awk '/control-plane/ {print $1; exit}')
	test -n "$node"
	echo "Restarting disposable control-plane node $node"
	docker stop "$node" >/dev/null
	docker start "$node" >/dev/null
	kubectl wait node --all --for=condition=Ready --timeout=180s
	wait_for_deployment_rollout "$replicas"
	wait_http /healthz
fi

echo "Kind production smoke test passed."
