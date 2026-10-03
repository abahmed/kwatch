#!/bin/sh

set -eu
# pipefail is not POSIX; enable it where the shell supports it.
# shellcheck disable=SC3040
(set -o pipefail) 2>/dev/null && set -o pipefail

default_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
harness_root=${KWATCH_HARNESS_ROOT:-${KWATCH_REPO_ROOT:-$default_root}}
candidate_root=${KWATCH_CANDIDATE_ROOT:-$harness_root}
cd "$harness_root"

: "${KIND_CLUSTER_NAME:=kwatch-e2e}"
: "${ARTIFACTS:=$(mktemp -d)}"
: "${KWATCH_IMAGE:=kwatch:e2e}"
: "${KWATCH_E2E:=true}"
: "${KEEP_CLUSTER:=false}"
: "${SUITE_TIMEOUT:=60m}"
: "${SOURCE_REF:=main}"
: "${BUILD_DATE:=$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
: "${KWATCH_CONFIG_FILE:=test/e2e/testdata/base-config.yaml}"
: "${METRICS_SERVER_VERSION:=v0.7.2}"
default_metrics_checksum=f103539a54ed72efe66616afc74a8bfaed651703cb391879\
7599046af5617441
: "${METRICS_SERVER_MANIFEST_SHA256:=$default_metrics_checksum}"

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
sanitize_bin=""
metrics_manifest=""
kubeconfig_file=""

require_tool() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "required tool is missing: $1" >&2
		exit 2
	fi
}

# json_escape prints $1 as a JSON string body: backslash, quote and control
# characters are escaped. jq is used when present.
json_escape() {
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$1" | jq -Rrs '@json | .[1:-1]'
		return
	fi
	printf '%s' "$1" | awk '
		BEGIN { ORS = "" }
		{
			if (NR > 1) printf "\\n"
			gsub(/\\/, "\\\\")
			gsub(/"/, "\\\"")
			gsub(/\t/, "\\t")
			gsub(/\r/, "\\r")
			print
		}'
}

# family_regex prints an anchored -run pattern of every covered test whose
# coverage.yaml entry has the given family, or nothing for an unknown one.
family_regex() {
	awk -v want="$1" '
		function flush() {
			if (family == want && status == "covered" && test != "" &&
				!(test in seen)) {
				seen[test] = 1
				names = names (names == "" ? "" : "|") test
			}
			family = ""; status = ""; test = ""
		}
		/^  - id:/ { flush() }
		/^    family:/ { family = $2 }
		/^    status:/ { status = $2 }
		/^    test:/ { test = $2 }
		END {
			flush()
			if (names != "") print "^(" names ")$"
		}
	' "$harness_root/test/e2e/coverage/coverage.yaml"
}

collect_diagnostics() {
	mkdir -p "$ARTIFACTS"
	requested_ref_json=$(json_escape "$SOURCE_REF")
	resolved_sha_json=$(json_escape "${source_sha:-unknown}")
	cluster_name_json=$(json_escape "$KIND_CLUSTER_NAME")
	kwatch_image_json=$(json_escape "$KWATCH_IMAGE")
	reported_version_json=$(json_escape "${REPORTED_VERSION:-}")
	config_hash=$(sha256sum "$KWATCH_CONFIG_FILE" 2>/dev/null |
		awk '{print $1}' || true)
	cat >"$ARTIFACTS/metadata.json" <<EOF
{
  "requestedRef": "$requested_ref_json",
  "resolvedSHA": "$resolved_sha_json",
  "clusterName": "$cluster_name_json",
  "kwatchImage": "$kwatch_image_json",
  "reportedVersion": "$reported_version_json",
  "configurationHash": "$config_hash",
  "startedAt": "${started_at:-unknown}"
}
EOF
	cat >"$ARTIFACTS/metadata.env" <<EOF
requested_ref=$SOURCE_REF
resolved_sha=${source_sha:-unknown}
cluster_name=$KIND_CLUSTER_NAME
kwatch_image=$KWATCH_IMAGE
kwatch_config_file=$KWATCH_CONFIG_FILE
reported_version=${REPORTED_VERSION:-}
configuration_hash=$config_hash
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
	kubectl get pods -A -o yaml 2>&1 | sanitize_to "$ARTIFACTS/pods.yaml" || true
	kubectl get events -A --sort-by=.lastTimestamp -o yaml \
		2>&1 | sanitize_to "$ARTIFACTS/events.yaml" || true
	kubectl get nodes -o yaml 2>&1 | sanitize_to "$ARTIFACTS/nodes.yaml" || true
	kubectl get leases -A -o yaml 2>&1 | \
		sanitize_to "$ARTIFACTS/leases.yaml" || true
	kubectl -n kwatch get deployment,serviceaccount,role,rolebinding \
		-o yaml 2>&1 | sanitize_to "$ARTIFACTS/kwatch-resources.yaml" || true
	kubectl -n kwatch get deployment kwatch -o yaml 2>&1 | \
		sanitize_to "$ARTIFACTS/kwatch-deployment.yaml" || true
	kubectl -n kwatch get configmaps -o yaml 2>&1 | \
		sanitize_to "$ARTIFACTS/kwatch-configmaps.yaml" || true
	for pod in $(kubectl -n kwatch get pods -l app=kwatch \
		-o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
		2>/dev/null || true); do
		kubectl -n kwatch logs "$pod" 2>&1 | \
			sanitize_to "$ARTIFACTS/kwatch-${pod}.log" || true
		kubectl -n kwatch logs "$pod" --previous 2>&1 | \
			sanitize_to "$ARTIFACTS/kwatch-${pod}-previous.log" || true
	done
	for resource in deployment serviceaccount role rolebinding; do
		kubectl -n kwatch describe "$resource" 2>&1 | \
			sanitize_to "$ARTIFACTS/kwatch-${resource}.describe" || true
	done
	capture_http_diagnostics
	kind export logs "$ARTIFACTS/kind-logs" \
		--name "$KIND_CLUSTER_NAME" >/dev/null 2>&1 || true
	if ! "$harness_root/scripts/check-e2e-artifacts.sh" "$ARTIFACTS"; then
		quarantine_unsafe_artifacts
	fi
}

quarantine_unsafe_artifacts() {
	quarantine_log="$ARTIFACTS/artifact-safety-quarantine.txt"
	printf '%s\n' \
		'artifact safety check found one or more unsafe files' \
		>"$quarantine_log"
	while IFS= read -r artifact; do
		if ! "$harness_root/scripts/check-e2e-artifacts.sh" "$artifact";
		then
			printf 'removed=%s\n' "${artifact#"$ARTIFACTS"/}" \
				>>"$quarantine_log"
			rm -f "$artifact"
		fi
	done <<EOF
$(find "$ARTIFACTS" -type f ! -name 'artifact-safety-quarantine.txt')
EOF
}

capture_http_diagnostics() {
	mkdir -p "$ARTIFACTS/kwatch" "$ARTIFACTS/receiver"
	receiver_pod=$(kubectl -n kwatch-e2e-system get pods \
		-l app=kwatch-e2e-receiver -o jsonpath='{.items[0].metadata.name}' \
		2>/dev/null || true)
	if [ -n "$receiver_pod" ]; then
		kubectl -n kwatch-e2e-system port-forward "pod/$receiver_pod" \
		18080:8080 >"$ARTIFACTS/receiver/port-forward.log" 2>&1 &
		receiver_forward=$!
		wait_for_http http://127.0.0.1:18080/healthz || true
		curl -fsS http://127.0.0.1:18080/requests 2>&1 | \
			sanitize_to "$ARTIFACTS/receiver/requests.json" || true
		curl -fsS http://127.0.0.1:18080/control 2>&1 | \
			sanitize_to "$ARTIFACTS/receiver/control.json" || true
		kill "$receiver_forward" >/dev/null 2>&1 || true
	fi
	find_kwatch_leader
	if [ -z "$leader_pod" ]; then
		return
	fi
	printf '%s\n' "$leader_pod" >"$ARTIFACTS/kwatch/leader.txt"
	kubectl -n kwatch port-forward "pod/$leader_pod" 18081:8060 \
		>"$ARTIFACTS/kwatch/port-forward.log" 2>&1 &
	kwatch_forward=$!
	wait_for_http http://127.0.0.1:18081/healthz || true
	for endpoint in healthz readyz availabilityz health metrics; do
		curl -fsS "http://127.0.0.1:18081/$endpoint" 2>&1 | \
			sanitize_to "$ARTIFACTS/kwatch/$endpoint" || true
	done
	kill "$kwatch_forward" >/dev/null 2>&1 || true
}

find_kwatch_leader() {
	leader_pod=""
	port=18082
	for pod in $(kubectl -n kwatch get pods -l app=kwatch \
		-o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
		2>/dev/null || true); do
		kubectl -n kwatch port-forward "pod/$pod" "$port:8060" \
			>/dev/null 2>&1 &
		forward=$!
		if wait_for_http "http://127.0.0.1:$port/readyz" &&
			curl -fsS "http://127.0.0.1:$port/readyz" \
			>/dev/null 2>&1; then
			leader_pod=$pod
			kill "$forward" >/dev/null 2>&1 || true
			return
		fi
		kill "$forward" >/dev/null 2>&1 || true
		port=$((port + 1))
	done
}

sanitize_to() {
	if [ -z "$sanitize_bin" ]; then
		return 1
	fi
	"$sanitize_bin" >"$1"
}

wait_for_http() {
	url=$1
	attempt=0
	while [ "$attempt" -lt 20 ]; do
		if curl -fsS "$url" >/dev/null 2>&1; then
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
	return 1
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
	if [ -n "$sanitize_bin" ]; then
		rm -f "$sanitize_bin"
	fi
	if [ -n "$metrics_manifest" ]; then
		rm -f "$metrics_manifest"
	fi
	if [ -n "$kubeconfig_file" ]; then
		rm -f "$kubeconfig_file"
	fi
	exit "$status"
}

# Validate the selection before any image or cluster work.
if [ -n "${SCENARIO_FAMILY:-}" ]; then
	if [ -n "${SCENARIO_REGEX:-}" ] || [ -n "${SCENARIOS:-}" ]; then
		echo "SCENARIO_FAMILY cannot be combined with SCENARIO_REGEX" \
			"or SCENARIOS" >&2
		exit 2
	fi
	if [ -z "$(family_regex "$SCENARIO_FAMILY")" ]; then
		echo "unknown SCENARIO_FAMILY: $SCENARIO_FAMILY" >&2
		exit 2
	fi
fi

trap cleanup EXIT INT TERM

require_tool docker
require_tool kind
require_tool kubectl
require_tool go
require_tool curl
sanitize_bin=$(mktemp)
go build -o "$sanitize_bin" ./cmd/kwatch-e2e-sanitize

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
source_sha=${KWATCH_SOURCE_SHA:-$(git rev-parse HEAD)}
mkdir -p "$ARTIFACTS"
for required_file in Dockerfile deploy/crd.yaml deploy/deploy.yaml; do
	if [ ! -f "$candidate_root/$required_file" ]; then
		echo "candidate is missing $required_file" >&2
		exit 2
	fi
done
if [ "$KWATCH_IMAGE" = kwatch:e2e ]; then
	KWATCH_IMAGE="kwatch:e2e-${source_sha}"
	docker build --load --tag "$KWATCH_IMAGE" \
		--build-arg RELEASE_VERSION=e2e \
		--build-arg GIT_COMMIT="$source_sha" \
		--build-arg BUILD_DATE="$BUILD_DATE" "$candidate_root"
	built_images="$KWATCH_IMAGE"
fi

receiver_image="kwatch-e2e-receiver:${source_sha}"
workload_image="kwatch-e2e-workload:${source_sha}"
receiver_host="kwatch-e2e-receiver.kwatch-e2e-system.svc.cluster.local"
receiver_url="http://${receiver_host}:8080/webhook"
docker build --load --tag "$receiver_image" test/e2e/receiver
docker build --load --tag "$workload_image" test/e2e/workload
built_images="$built_images $receiver_image $workload_image"

kind create cluster \
	--name "$KIND_CLUSTER_NAME" \
	--config test/e2e/testdata/kind-config.yaml
kubeconfig_file=$(mktemp)
kind get kubeconfig --name "$KIND_CLUSTER_NAME" >"$kubeconfig_file"
export KUBECONFIG="$kubeconfig_file"
kind load docker-image "$KWATCH_IMAGE" --name "$KIND_CLUSTER_NAME"
kind load docker-image "$receiver_image" --name "$KIND_CLUSTER_NAME"
kind load docker-image "$workload_image" --name "$KIND_CLUSTER_NAME"

metrics_manifest=$(mktemp)
curl -fsSL "https://github.com/kubernetes-sigs/metrics-server/releases/\
download/$METRICS_SERVER_VERSION/components.yaml" -o "$metrics_manifest"
metrics_checksum=$(sha256sum "$metrics_manifest" | awk '{print $1}')
if [ "$metrics_checksum" != "$METRICS_SERVER_MANIFEST_SHA256" ]; then
	echo "metrics-server manifest checksum mismatch" >&2
	exit 1
fi
kubectl apply -f "$metrics_manifest"
metrics_patch='[{"op":"add","path":'
metrics_patch="${metrics_patch}\"/spec/template/spec/containers/0/args/-\","
metrics_patch="${metrics_patch}\"value\":\"--kubelet-insecure-tls\"}]"
kubectl -n kube-system patch deployment metrics-server --type=json \
	-p "$metrics_patch"
kubectl -n kube-system rollout status deployment/metrics-server \
	--timeout=5m
kubectl wait --for=condition=Available \
	apiservice/v1beta1.metrics.k8s.io --timeout=5m

kubectl apply -f "$candidate_root/deploy/crd.yaml"
kubectl wait --for=condition=Established \
	crd/kwatchconfigs.kwatch.abahmed.dev --timeout=120s
kubectl create namespace kwatch --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic kwatch \
	--namespace kwatch \
	--from-file=config.yaml="$KWATCH_CONFIG_FILE" \
	--from-literal=webhook-url="$receiver_url" \
	--dry-run=client -o yaml | kubectl apply -f -
# Rewrite the image before the first apply so the released image never starts.
kwatch_manifest="$ARTIFACTS/kwatch-deploy.yaml"
image_expr="s#^\([[:space:]]*image:\) ghcr.io/abahmed/kwatch:.*#\1 $KWATCH_IMAGE#"
pull_expr='s#^\([[:space:]]*imagePullPolicy:\) IfNotPresent#\1 Never#'
mkdir -p "$ARTIFACTS"
sed -e "$image_expr" -e "$pull_expr" \
	"$candidate_root/deploy/deploy.yaml" >"$kwatch_manifest"
if ! grep -q "image: $KWATCH_IMAGE\$" "$kwatch_manifest" ||
	! grep -q 'imagePullPolicy: Never$' "$kwatch_manifest"; then
	echo "failed to substitute the candidate image into deploy.yaml" >&2
	exit 1
fi
kubectl apply -f "$kwatch_manifest"
# KWATCH_VERBOSITY=4 makes Kwatch log every finding and incident membership
# change, which explains an incident that does not resolve.
if [ -n "${KWATCH_VERBOSITY:-}" ]; then
	args_path=/spec/template/spec/containers/0/args
	verbosity_patch="[{\"op\":\"add\",\"path\":\"$args_path\","
	verbosity_patch="$verbosity_patch\"value\":[\"-v=$KWATCH_VERBOSITY\"]}]"
	kubectl --namespace kwatch patch deployment kwatch --type json \
		-p "$verbosity_patch"
fi
sed "s#kwatch-e2e-receiver:e2e#$receiver_image#g" \
	test/e2e/receiver/deployment.yaml | kubectl apply -f -
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
# A family comes from the family field of test/e2e/coverage/coverage.yaml,
# so the selection cannot drift from the coverage catalog.
if [ -n "${SCENARIO_FAMILY:-}" ]; then
	scenario_regex=$(family_regex "$SCENARIO_FAMILY")
	if [ -z "$scenario_regex" ]; then
		echo "unknown SCENARIO_FAMILY: $SCENARIO_FAMILY" >&2
		exit 2
	fi
fi

set +e
KWATCH_E2E="$KWATCH_E2E" \
	KWATCH_EXTENDED=true \
	KWATCH_IMAGE="$KWATCH_IMAGE" \
	WORKLOAD_IMAGE="$workload_image" \
	SCENARIO_TIMEOUT="${SCENARIO_TIMEOUT:-20m}" \
	SCENARIO_FAMILY="${SCENARIO_FAMILY:-}" \
	SCENARIO_SHARD="${SCENARIO_SHARD:-}" \
	go test -tags=e2e -count=1 -v ./test/e2e/... \
		-timeout "$SUITE_TIMEOUT" \
		-parallel "${SCENARIO_PARALLEL:-6}" \
		-run "$scenario_regex" \
		>"$ARTIFACTS/go-test.log" 2>&1
test_status=$?
set -e
cat "$ARTIFACTS/go-test.log"
# One line per scenario with its result and duration, slowest first; use it
# to update the minutes in test/e2e/coverage/coverage.yaml.
echo "Scenario timings:"
grep -E -- '^--- (PASS|FAIL|SKIP): TestScenario' "$ARTIFACTS/go-test.log" |
	sed -E 's/^--- ([A-Z]+): ([A-Za-z0-9]+) \(([0-9.]+)s\)/\3 \1 \2/' |
	sort -rn | awk '{printf "  %4.0fs %-4s %s\n", $1, $2, $3}' || true
cleanup "$test_status"
