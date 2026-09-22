#!/bin/sh

set -eu

root_dir=${KWATCH_REPO_ROOT:-$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)}
cd "$root_dir"

: "${KIND_CLUSTER_NAME:=kwatch-e2e}"
: "${ARTIFACTS:=$(mktemp -d)}"
: "${KWATCH_IMAGE:=kwatch:e2e}"
: "${KWATCH_E2E:=true}"
: "${KEEP_CLUSTER:=false}"
: "${SUITE_TIMEOUT:=60m}"
: "${KWATCH_REPLICAS:=2}"
: "${SOURCE_REF:=main}"
: "${BUILD_DATE:=$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
: "${KWATCH_CONFIG_FILE:=test/e2e/testdata/base-config.yaml}"
: "${METRICS_SERVER_VERSION:=v0.7.2}"

install_only=false
case "${1:-}" in
--install-only)
	install_only=true
	;;
"")
	;;
*)
	echo "usage: $0 [--install-only]" >&2
	exit 2
	;;
esac

status=0
built_images=""

require_tool() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "required tool is missing: $1" >&2
		exit 2
	}
}

collect_diagnostics() {
	mkdir -p "$ARTIFACTS"
	json_escape() {
		printf '%s' "$1" | sed 's/[\\&]/\\&\\&/g; s/"/\\\\"/g'
	}
	requested_ref_json=$(json_escape "$SOURCE_REF")
	resolved_sha_json=$(json_escape "${source_sha:-unknown}")
	cluster_name_json=$(json_escape "$KIND_CLUSTER_NAME")
	kwatch_image_json=$(json_escape "$KWATCH_IMAGE")
	cat >"$ARTIFACTS/metadata.json" <<EOF
{
  "requestedRef": "$requested_ref_json",
  "resolvedSHA": "$resolved_sha_json",
  "clusterName": "$cluster_name_json",
  "kwatchImage": "$kwatch_image_json",
  "startedAt": "${started_at:-unknown}"
}
EOF
	cat >"$ARTIFACTS/metadata.env" <<EOF
requested_ref=$SOURCE_REF
resolved_sha=${source_sha:-unknown}
cluster_name=$KIND_CLUSTER_NAME
kwatch_image=$KWATCH_IMAGE
kwatch_config_file=$KWATCH_CONFIG_FILE
scenario_regex=${SCENARIO_REGEX:-TestScenario}
scenario_family=${SCENARIO_FAMILY:-}
scenario_shard=${SCENARIO_SHARD:-}
started_at=${started_at:-unknown}
EOF
	cat >"$ARTIFACTS/results.json" <<EOF
{
  "exitCode": ${status:-2},
  "passed": $([ "${status:-2}" -eq 0 ] && printf true || printf false),
  "finishedAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
	kubectl get pods -A -o yaml >"$ARTIFACTS/pods.yaml" 2>&1 || true
	kubectl get events -A --sort-by=.lastTimestamp -o yaml \
		>"$ARTIFACTS/events.yaml" 2>&1 || true
	kubectl get nodes -o yaml >"$ARTIFACTS/nodes.yaml" 2>&1 || true
	kubectl get leases -A -o yaml >"$ARTIFACTS/leases.yaml" 2>&1 || true
	kubectl -n kwatch get deployment,serviceaccount,role,rolebinding \
		-o yaml >"$ARTIFACTS/kwatch-resources.yaml" 2>&1 || true
	kubectl -n kwatch get deployment kwatch -o yaml \
		>"$ARTIFACTS/kwatch-deployment.yaml" 2>&1 || true
	for pod in $(kubectl -n kwatch get pods -l app=kwatch \
		-o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
		2>/dev/null || true); do
		kubectl -n kwatch logs "$pod" \
			>"$ARTIFACTS/kwatch-${pod}.log" 2>&1 || true
		kubectl -n kwatch logs "$pod" --previous \
			>"$ARTIFACTS/kwatch-${pod}-previous.log" 2>&1 || true
	done
	for resource in deployment serviceaccount role rolebinding; do
		kubectl -n kwatch describe "$resource" \
			>"$ARTIFACTS/kwatch-${resource}.describe" 2>&1 || true
	done
	kind export logs "$ARTIFACTS/kind-logs" \
		--name "$KIND_CLUSTER_NAME" >/dev/null 2>&1 || true
}

cleanup() {
	status=${1:-$?}
	trap - EXIT INT TERM
	collect_diagnostics
	if [ "$KEEP_CLUSTER" != true ]; then
		kind delete cluster --name "$KIND_CLUSTER_NAME" >/dev/null 2>&1 || true
	fi
	for image in $built_images; do
		docker image rm "$image" >/dev/null 2>&1 || true
	done
	exit "$status"
}

trap cleanup EXIT INT TERM

require_tool docker
require_tool kind
require_tool kubectl
require_tool go

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
source_sha=${KWATCH_SOURCE_SHA:-$(git rev-parse HEAD)}
mkdir -p "$ARTIFACTS"
if [ "$KWATCH_IMAGE" = kwatch:e2e ]; then
	KWATCH_IMAGE="kwatch:e2e-${source_sha}"
	docker build --load --tag "$KWATCH_IMAGE" \
		--build-arg RELEASE_VERSION=e2e \
		--build-arg GIT_COMMIT="$source_sha" \
		--build-arg BUILD_DATE="$BUILD_DATE" .
	built_images="$KWATCH_IMAGE"
fi

receiver_image="kwatch-e2e-receiver:${source_sha}"
workload_image="kwatch-e2e-workload:${source_sha}"
docker build --load --tag "$receiver_image" test/e2e/receiver
docker build --load --tag "$workload_image" test/e2e/workload
built_images="$built_images $receiver_image $workload_image"

kind create cluster \
	--name "$KIND_CLUSTER_NAME" \
	--config test/e2e/testdata/kind-config.yaml
kind load docker-image "$KWATCH_IMAGE" --name "$KIND_CLUSTER_NAME"
kind load docker-image "$receiver_image" --name "$KIND_CLUSTER_NAME"
kind load docker-image "$workload_image" --name "$KIND_CLUSTER_NAME"

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
kubectl create secret generic kwatch \
	--namespace kwatch \
	--from-file=config.yaml="$KWATCH_CONFIG_FILE" \
	--from-literal=diagnostics-token=e2e-token \
	--dry-run=client -o yaml | kubectl apply -f -
sed "s#kwatch-e2e-receiver:e2e#$receiver_image#g" \
	test/e2e/receiver/deployment.yaml | kubectl apply -f -
kubectl -n kwatch set image deployment/kwatch \
		kwatch="$KWATCH_IMAGE"
kubectl -n kwatch scale deployment/kwatch --replicas="$KWATCH_REPLICAS"
kubectl -n kwatch rollout status deployment/kwatch --timeout=10m
kubectl -n kwatch annotate deployment/kwatch \
		"kwatch.e2e/source-sha=$source_sha" --overwrite
actual_image=$(kubectl -n kwatch get deployment/kwatch \
		-o jsonpath='{.spec.template.spec.containers[?(@.name=="kwatch")].image}')
if [ "$actual_image" != "$KWATCH_IMAGE" ]; then
	echo "Kwatch image mismatch: expected $KWATCH_IMAGE, got $actual_image" >&2
	exit 1
fi
kubectl -n kwatch-e2e-system rollout status \
	deployment/kwatch-e2e-receiver --timeout=5m

if [ "$install_only" = true ]; then
	cleanup 0
fi

scenario_regex="${SCENARIO_REGEX:-TestScenario}"
if [ -n "${SCENARIOS:-}" ]; then
	scenario_regex=""
	old_ifs=$IFS
	IFS=,
	for scenario in $SCENARIOS; do
		case "$scenario" in
		pod.crashloop|pod-crashloop)
			name=TestScenarioPodCrashLoop
			;;
		*)
			name="$scenario"
			;;
		esac
		if [ -n "$scenario_regex" ]; then
			scenario_regex="$scenario_regex|$name"
		else
			scenario_regex="$name"
		fi
	done
	IFS=$old_ifs
fi
if [ -n "${SCENARIO_FAMILY:-}" ]; then
	case "$SCENARIO_FAMILY" in
	pod) scenario_regex="TestScenarioPod" ;;
	grouping) scenario_regex="TestScenarioGrouping" ;;
	lifecycle) scenario_regex="TestScenario(Resolution|Restart|Leader|Provider|Configuration)" ;;
	node) scenario_regex="TestScenarioNode" ;;
	workload) scenario_regex="TestScenario(Deployment|Job|CronJob|StatefulSet|DaemonSet|PDB|ReplicaSet)" ;;
	storage) scenario_regex="TestScenario(PersistentVolumeClaim|ExtendedVolumeAttachment)" ;;
	networking) scenario_regex="TestScenario(Service|MissingIngress)" ;;
	security) scenario_regex="TestScenario(Missing|InvalidStartup|ExtendedAdmission)" ;;
	integration) scenario_regex="TestScenario(ActiveProbe|Heartbeat|NodeRecovery|ControlPlane|ExtendedTLS|ExtendedMetrics)" ;;
	invalid-config) scenario_regex="TestScenarioInvalid" ;;
	*) echo "unknown SCENARIO_FAMILY: $SCENARIO_FAMILY" >&2; exit 2 ;;
	esac
fi

set +e
KWATCH_E2E="$KWATCH_E2E" \
	KWATCH_EXTENDED=true \
	KWATCH_IMAGE="$KWATCH_IMAGE" \
	WORKLOAD_IMAGE="$workload_image" \
	SCENARIO_TIMEOUT="${SCENARIO_TIMEOUT:-10m}" \
	SCENARIO_FAMILY="${SCENARIO_FAMILY:-}" \
	SCENARIO_SHARD="${SCENARIO_SHARD:-}" \
	go test -tags=e2e -count=1 ./test/e2e/... \
		-timeout "$SUITE_TIMEOUT" \
		-run "$scenario_regex" \
		>"$ARTIFACTS/go-test.log" 2>&1
test_status=$?
set -e
cat "$ARTIFACTS/go-test.log"
cleanup "$test_status"
