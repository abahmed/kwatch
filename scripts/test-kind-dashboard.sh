#!/usr/bin/env bash
set -euo pipefail

# Disposable-cluster check of the dashboard as the chart installs it: the
# web UI and API on their own Service, password sign-in with a session
# cookie, the cookie on /api/*, sign-out, and shared mode refusing sign-in
# over plain http. The image must be built with the licensetest tag; it
# runs as Pro through KWATCH_EDITION_OVERRIDE.
release="${KWATCH_RELEASE:-kwatch-dash}"
namespace="${KWATCH_NAMESPACE:-kwatch-dash}"
cluster="${KIND_CLUSTER_NAME:-kwatch-ops}"
image="${KWATCH_IMAGE:-kwatch:ci}"
port=18061

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
# shellcheck source=scripts/require-kind-context.sh
. "$root_dir/scripts/require-kind-context.sh"
require_kind_context "kind-$cluster" || exit 2

work=$(mktemp -d)
cleanup() {
	kill "${forward_pid:-}" >/dev/null 2>&1 || true
	helm uninstall "$release" --namespace "$namespace" >/dev/null 2>&1 || true
	kubectl delete namespace "$namespace" --ignore-not-found \
		--wait=false >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT

fail() {
	echo "FAIL: $*" >&2
	kubectl --namespace "$namespace" logs "deployment/$release" \
		--tail=80 >&2 || true
	cat "$work/forward.log" >&2 2>/dev/null || true
	exit 1
}

chart_values=(
	--set image.repository="${image%%:*}"
	--set image.tag="${image##*:}"
	--set image.pullPolicy=Never
	--set config.telemetry.enabled=false
	--set dashboard.enabled=true
)

rollout() {
	kubectl --namespace "$namespace" set env "deployment/$release" \
		KWATCH_EDITION_OVERRIDE=pro >/dev/null
	kubectl --namespace "$namespace" rollout status \
		"deployment/$release" --timeout=240s
}

start_forward() {
	kill "${forward_pid:-}" >/dev/null 2>&1 || true
	kubectl --namespace "$namespace" port-forward \
		"service/$release-dashboard" "$port:8061" \
		>"$work/forward.log" 2>&1 &
	forward_pid=$!
	for _ in $(seq 1 30); do
		if curl --silent --output /dev/null "http://127.0.0.1:$port/"; then
			return 0
		fi
		sleep 1
	done
	fail "the dashboard port-forward did not open"
}

# code prints the HTTP status of a request; extra curl arguments follow.
code() {
	curl --silent --output "$work/body" --write-out '%{http_code}' \
		--dump-header "$work/headers" "$@"
}

login() {
	code --cookie-jar "$work/jar" --header 'Content-Type: application/json' \
		--data "{\"password\":\"$1\"}" "http://127.0.0.1:$port/login"
}

echo "Installing $release with the dashboard on (local mode)"
kubectl create namespace "$namespace" --dry-run=client -o yaml |
	kubectl apply -f - >/dev/null
kind load docker-image "$image" --name "$cluster"
helm install "$release" deploy/chart --namespace "$namespace" \
	"${chart_values[@]}" --wait=false
rollout

password=$(kubectl --namespace "$namespace" get secret \
	"$release-dashboard" -o jsonpath='{.data.password}' | base64 --decode)
[[ "$password" =~ ^([A-HJ-NP-Z2-9]{4}-){3}[A-HJ-NP-Z2-9]{4}$ ]] ||
	fail "the generated password has the wrong shape"

start_forward
[ "$(code "http://127.0.0.1:$port/")" = 200 ] || fail "the UI shell"
grep -qi "^content-security-policy: default-src 'none'" "$work/headers" ||
	fail "the UI shell has no strict CSP"
[ "$(code "http://127.0.0.1:$port/api/incidents")" = 401 ] ||
	fail "/api/incidents without a session must answer 401"
[ "$(login wrong-password)" = 401 ] || fail "a wrong password"

[ "$(login "$password")" = 204 ] || fail "sign-in with the Secret's password"
grep -i '^set-cookie: kwatch_session=' "$work/headers" |
	grep -i 'httponly' | grep -qi 'samesite=strict' ||
	fail "the session cookie lacks HttpOnly or SameSite=Strict"

# The pipeline answers 503 until it is up; wait for it with the cookie.
status=""
for _ in $(seq 1 60); do
	status=$(code --cookie "$work/jar" \
		"http://127.0.0.1:$port/api/incidents?state=active")
	[ "$status" = 200 ] && break
	sleep 3
done
[ "$status" = 200 ] || fail "/api/incidents with the cookie answered $status"
grep -q '"incidents":' "$work/body" || fail "the incident list body"
[ "$(code --cookie "$work/jar" "http://127.0.0.1:$port/api/status")" = 200 ] ||
	fail "/api/status with the cookie"
grep -q "\"kwatchNamespace\":\"$namespace\"" "$work/body" ||
	fail "/api/status does not name kwatch's namespace"

[ "$(code --cookie "$work/jar" --cookie-jar "$work/jar" --request POST \
	"http://127.0.0.1:$port/logout")" = 204 ] || fail "sign-out"
[ "$(code --cookie "$work/jar" "http://127.0.0.1:$port/api/session")" = 401 ] ||
	fail "the session must end at sign-out"

echo "Switching to shared mode: sign-in over plain http is refused"
helm upgrade "$release" deploy/chart --namespace "$namespace" \
	"${chart_values[@]}" --set dashboard.expose=ingress \
	--set dashboard.host=kwatch.example.test \
	--set dashboard.ingress.tlsSecretName=kwatch-dashboard-tls \
	--wait=false
rollout
kubectl --namespace "$namespace" get ingress "$release-dashboard" \
	>/dev/null || fail "shared mode did not create the Ingress"
start_forward
if [ "$(login "$password")" != 403 ] ||
	! grep -q https_required "$work/body"; then
	fail "shared mode must refuse sign-in over plain http"
fi

echo "Kind dashboard test passed."
