#!/usr/bin/env bash
set -euo pipefail

# Disposable-cluster check of the dashboard as the chart installs it: the
# web UI and API on their own Service, password sign-in with a session
# cookie, the cookie on /api/*, sign-out, and shared mode refusing sign-in
# over plain http. The password must survive a re-render and a helm upgrade,
# forwarded headers count only from trusted proxies, and Community refuses
# the dashboard. The image must be built with the licensetest tag; it
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
		"http://127.0.0.1:$port/api/incidents?status=active")
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
[ "$(code --cookie "$work/jar" "http://127.0.0.1:$port/api/incidents")" = 401 ] ||
	fail "the session must end at sign-out"

echo "The password survives a re-render and a helm upgrade"
helm template "$release" deploy/chart --namespace "$namespace" \
	"${chart_values[@]}" >"$work/render1.yaml"
helm template "$release" deploy/chart --namespace "$namespace" \
	"${chart_values[@]}" >"$work/render2.yaml"
cmp -s "$work/render1.yaml" "$work/render2.yaml" ||
	fail "two renders of the chart differ"
grep -Eq '^  (password|token): ' "$work/render1.yaml" &&
	fail "the render carries credentials"
helm upgrade "$release" deploy/chart --namespace "$namespace" \
	"${chart_values[@]}" --wait=false
rollout
again=$(kubectl --namespace "$namespace" get secret \
	"$release-dashboard" -o jsonpath='{.data.password}' | base64 --decode)
[ "$again" = "$password" ] || fail "the password changed after helm upgrade"
start_forward
[ "$(login "$password")" = 204 ] ||
	fail "the saved password stopped working after the upgrade"

echo "Forwarded headers count only from trusted proxies"
# Through a port-forward the peer is 127.0.0.1.
helm upgrade "$release" deploy/chart --namespace "$namespace" \
	"${chart_values[@]}" \
	--set 'dashboard.trustedProxies[0]=10.255.255.0/24' --wait=false
rollout
start_forward
if [ "$(code --header 'X-Forwarded-Proto: https' \
	--header 'Content-Type: application/json' \
	--data "{\"password\":\"$password\"}" \
	"http://127.0.0.1:$port/login")" != 403 ] ||
	! grep -q https_required "$work/body"; then
	fail "X-Forwarded-Proto from an untrusted peer must not count"
fi
helm upgrade "$release" deploy/chart --namespace "$namespace" \
	"${chart_values[@]}" \
	--set 'dashboard.trustedProxies[0]=127.0.0.1/32' --wait=false
rollout
start_forward
[ "$(login "$password")" = 403 ] ||
	fail "a trusted proxy without X-Forwarded-Proto is http: refuse"
[ "$(code --cookie-jar "$work/jar" --header 'X-Forwarded-Proto: https' \
	--header 'X-Forwarded-For: 203.0.113.9' \
	--header 'Content-Type: application/json' \
	--data "{\"password\":\"$password\"}" \
	"http://127.0.0.1:$port/login")" = 204 ] ||
	fail "a trusted proxy that says https must be believed"

echo "Community refuses the dashboard"
kubectl --namespace "$namespace" set env "deployment/$release" \
	KWATCH_EDITION_OVERRIDE- >/dev/null
kubectl --namespace "$namespace" rollout status "deployment/$release" \
	--timeout=240s
start_forward_expect_closed() {
	kill "${forward_pid:-}" >/dev/null 2>&1 || true
	kubectl --namespace "$namespace" port-forward \
		"service/$release-dashboard" "$port:8061" \
		>"$work/forward.log" 2>&1 &
	forward_pid=$!
	sleep 5
	if curl --silent --max-time 5 --output /dev/null \
		"http://127.0.0.1:$port/"; then
		fail "Community must not open the dashboard port"
	fi
}
start_forward_expect_closed

echo "Kind dashboard test passed."
