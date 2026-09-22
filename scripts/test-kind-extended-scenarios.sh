#!/bin/sh

set -eu

root_dir=${KWATCH_REPO_ROOT:-$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)}
cd "$root_dir"

: "${KIND_CLUSTER_NAME:=kwatch-extended-e2e}"
: "${ARTIFACTS:=$(mktemp -d)}"
: "${SCENARIO_REGEX:=TestScenarioExtended}"
: "${SUITE_TIMEOUT:=60m}"
: "${SOURCE_REF:=main}"
: "${KWATCH_CONFIG_FILE:=test/e2e/testdata/base-config.yaml}"
: "${KEEP_CLUSTER:=false}"
: "${KWATCH_IMAGE:=}"
: "${RECEIVER_IMAGE:=}"
: "${WORKLOAD_IMAGE:=}"
: "${METRICS_SERVER_VERSION:=v0.7.2}"

status=0
built_images=""
cluster_created=false

collect_diagnostics() {
	mkdir -p "$ARTIFACTS"
	kubectl get pods -A -o yaml >"$ARTIFACTS/pods.yaml" 2>&1 || true
	kubectl get events -A --sort-by=.lastTimestamp -o yaml \
		>"$ARTIFACTS/events.yaml" 2>&1 || true
	kubectl get nodes -o yaml >"$ARTIFACTS/nodes.yaml" 2>&1 || true
	kubectl get leases -A -o yaml >"$ARTIFACTS/leases.yaml" 2>&1 || true
	kubectl -n kwatch get deployment -o yaml \
		>"$ARTIFACTS/kwatch-deployment.yaml" 2>&1 || true
	for pod in $(kubectl -n kwatch get pods -l app=kwatch \
		-o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
		2>/dev/null || true); do
		kubectl -n kwatch logs "$pod" \
			>"$ARTIFACTS/kwatch-${pod}.log" 2>&1 || true
		kubectl -n kwatch logs "$pod" --previous \
			>"$ARTIFACTS/kwatch-${pod}-previous.log" 2>&1 || true
	done
	kubectl -n kwatch-e2e-system get all -o yaml \
		>"$ARTIFACTS/receiver.yaml" 2>&1 || true
	kind export logs "$ARTIFACTS/kind-logs" \
		--name "$KIND_CLUSTER_NAME" >/dev/null 2>&1 || true
	cat >"$ARTIFACTS/metadata.env" <<EOF
requested_ref=$SOURCE_REF
resolved_sha=${KWATCH_SOURCE_SHA:-unknown}
cluster_name=$KIND_CLUSTER_NAME
kwatch_image=$KWATCH_IMAGE
receiver_image=$RECEIVER_IMAGE
workload_image=$WORKLOAD_IMAGE
scenario_regex=$SCENARIO_REGEX
kind_extended_suite=true
EOF
}

cleanup() {
	status=${1:-$?}
	trap - EXIT INT TERM
	if [ "$cluster_created" = true ]; then
		collect_diagnostics
		if [ "$KEEP_CLUSTER" != true ]; then
			kind delete cluster --name "$KIND_CLUSTER_NAME" \
				>/dev/null 2>&1 || true
		fi
	fi
	for image in $built_images; do
		docker image rm "$image" >/dev/null 2>&1 || true
	done
	exit "$status"
}

trap cleanup EXIT INT TERM

for tool in docker kind kubectl go; do
	command -v "$tool" >/dev/null 2>&1 || {
		echo "required tool is missing: $tool" >&2
		exit 2
	}
done

source_sha=${KWATCH_SOURCE_SHA:-$(git rev-parse HEAD)}
if [ -z "$KWATCH_IMAGE" ]; then
	KWATCH_IMAGE="kwatch:extended-$source_sha"
	docker build --load --tag "$KWATCH_IMAGE" \
		--build-arg RELEASE_VERSION=e2e \
		--build-arg GIT_COMMIT="$source_sha" \
		--build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" .
	built_images="$KWATCH_IMAGE"
fi
if [ -z "$RECEIVER_IMAGE" ]; then
	RECEIVER_IMAGE="kwatch-e2e-receiver:extended-$source_sha"
	docker build --load --tag "$RECEIVER_IMAGE" test/e2e/receiver
	built_images="$built_images $RECEIVER_IMAGE"
fi
if [ -z "$WORKLOAD_IMAGE" ]; then
	WORKLOAD_IMAGE="kwatch-e2e-workload:extended-$source_sha"
	docker build --load --tag "$WORKLOAD_IMAGE" test/e2e/workload
	built_images="$built_images $WORKLOAD_IMAGE"
fi

kind create cluster --name "$KIND_CLUSTER_NAME" \
	--config test/e2e/testdata/kind-config.yaml
cluster_created=true
kind load docker-image "$KWATCH_IMAGE" --name "$KIND_CLUSTER_NAME"
kind load docker-image "$RECEIVER_IMAGE" --name "$KIND_CLUSTER_NAME"
kind load docker-image "$WORKLOAD_IMAGE" --name "$KIND_CLUSTER_NAME"
kubectl cluster-info >/dev/null

kubectl apply -f "https://github.com/kubernetes-sigs/metrics-server/\
releases/download/$METRICS_SERVER_VERSION/components.yaml"
kubectl -n kube-system patch deployment metrics-server --type=json \
	-p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
kubectl -n kube-system rollout status deployment/metrics-server \
	--timeout=5m
kubectl wait --for=condition=Available \
	apiservice/v1beta1.metrics.k8s.io --timeout=5m

kubectl apply -f deploy/crd.yaml
kubectl wait --for=condition=Established \
	crd/kwatchconfigs.kwatch.abahmed.dev --timeout=120s
kubectl apply -k test/e2e/install
kubectl create secret generic kwatch --namespace kwatch \
	--from-file=config.yaml="$KWATCH_CONFIG_FILE" \
	--from-literal=diagnostics-token=e2e-token \
	--dry-run=client -o yaml | kubectl apply -f -
sed "s#kwatch-e2e-receiver:e2e#$RECEIVER_IMAGE#g" \
	test/e2e/receiver/deployment.yaml | kubectl apply -f -
kubectl -n kwatch set image deployment/kwatch kwatch="$KWATCH_IMAGE"
kubectl -n kwatch patch deployment/kwatch --type=json \
	-p='[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Never"}]'
kubectl -n kwatch rollout status deployment/kwatch --timeout=10m
kubectl -n kwatch-e2e-system rollout status \
	deployment/kwatch-e2e-receiver --timeout=5m

set +e
env \
	KWATCH_E2E=true \
	KWATCH_EXTENDED=true \
	KWATCH_IMAGE="$KWATCH_IMAGE" \
	WORKLOAD_IMAGE="$WORKLOAD_IMAGE" \
	SCENARIO_REGEX="$SCENARIO_REGEX" \
	SUITE_TIMEOUT="$SUITE_TIMEOUT" \
	go test -tags=e2e -count=1 ./test/e2e/... \
	-timeout "$SUITE_TIMEOUT" -run "$SCENARIO_REGEX" \
	>"$ARTIFACTS/go-test.log" 2>&1
status=$?
set -e
cat "$ARTIFACTS/go-test.log"
cleanup "$status"
