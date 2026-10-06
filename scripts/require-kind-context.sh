#!/bin/sh
# Source this file from a script that creates or deletes cluster resources,
# then call require_kind_context before the first create and before any EXIT
# trap that deletes things. It refuses to continue unless the current kubectl
# context belongs to a kind cluster (name starts with "kind-"), so a script
# started by mistake with a real context cannot touch that cluster.
#
# An optional argument names the exact context expected, for example
# "kind-kwatch-ops". KWATCH_E2E_ALLOW_ANY_CONTEXT=true skips the check. Use
# it only for a disposable cluster that is not created by kind.

require_kind_context() {
	if [ "${KWATCH_E2E_ALLOW_ANY_CONTEXT:-}" = true ]; then
		return 0
	fi
	current_context=$(kubectl config current-context 2>/dev/null || true)
	case "$current_context" in
	kind-?*)
		if [ -z "${1:-}" ] || [ "$current_context" = "$1" ]; then
			return 0
		fi
		;;
	esac
	echo "refusing to run: the current kubectl context is" \
		"'${current_context:-none}', not the kind context" \
		"'${1:-kind-*}'." >&2
	echo "Switch to a kind cluster or set" \
		"KWATCH_E2E_ALLOW_ANY_CONTEXT=true for a disposable cluster." >&2
	return 1
}
