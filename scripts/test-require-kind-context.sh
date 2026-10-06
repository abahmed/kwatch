#!/bin/sh
# Checks scripts/require-kind-context.sh against throwaway kubeconfig files.
# kubectl config edits only the local file; no cluster is contacted.
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

# shellcheck source=scripts/require-kind-context.sh
. "$root_dir/scripts/require-kind-context.sh"

make_kubeconfig() {
	file="$tmp_dir/$1"
	: >"$file"
	if [ -n "$2" ]; then
		kubectl config --kubeconfig "$file" set-context "$2" \
			--cluster=x --user=y >/dev/null
		kubectl config --kubeconfig "$file" use-context "$2" >/dev/null
	fi
	printf '%s\n' "$file"
}

expect() {
	want=$1
	name=$2
	shift 2
	if (KUBECONFIG=$1 KWATCH_E2E_ALLOW_ANY_CONTEXT=${2:-} \
		require_kind_context "${3:-}" >/dev/null 2>&1); then
		got=pass
	else
		got=refuse
	fi
	if [ "$got" != "$want" ]; then
		echo "FAIL: $name: got $got, want $want" >&2
		exit 1
	fi
}

kind_file=$(make_kubeconfig kind kind-kwatch-e2e)
prod_file=$(make_kubeconfig prod arn:aws:eks:eu-west-1:1:cluster/prod)
bare_file=$(make_kubeconfig bare "")

expect pass "kind context" "$kind_file"
expect refuse "non-kind context" "$prod_file"
expect refuse "no context" "$bare_file"
expect pass "explicit opt-in" "$prod_file" true
expect refuse "opt-in must be exactly true" "$prod_file" yes
expect pass "expected kind context" "$kind_file" "" kind-kwatch-e2e
expect refuse "other kind context" "$kind_file" "" kind-other
echo "kind context guard: PASS"
