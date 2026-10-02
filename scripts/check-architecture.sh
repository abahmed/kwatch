#!/bin/sh

# Enforce the dependency direction described in AGENTS.md. This is a small
# import-level guard, not a replacement for design review or Go compilation.
# It needs only POSIX tools (find and grep).
set -eu
# pipefail is not POSIX; enable it where the shell supports it.
(set -o pipefail) 2>/dev/null && set -o pipefail

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
cd "$root_dir"

status=0
module='github.com/abahmed/kwatch'

# production_files prints non-test Go files under the given directories,
# minus any path matching the optional exclusion regular expression.
production_files() {
	exclude=$1
	shift
	files=$(find "$@" -name '*.go' ! -name '*_test.go' 2>/dev/null | sort)
	if [ -n "$exclude" ] && [ -n "$files" ]; then
		files=$(printf '%s\n' "$files" | grep -Ev "$exclude" || true)
	fi
	printf '%s\n' "$files"
}

# report_matches label pattern exclude dir...
report_matches() {
	label=$1
	pattern=$2
	exclude=$3
	shift 3
	files=$(production_files "$exclude" "$@")
	if [ -z "$files" ]; then
		return 0
	fi
	# shellcheck disable=SC2086
	if matches=$(grep -nE -e "$pattern" $files); then
		echo "architecture violation: $label"
		echo "$matches"
		status=1
	fi
}

# forbid_imports label dir forbidden-alternation [exclude-regexp]
# Forbidden names are internal package paths, matched with any subpackage.
# The optional exclusion skips files under a subpackage with its own rule.
# A rule whose directory no longer exists fails: a renamed package would
# otherwise leave its rule silently checking nothing.
forbid_imports() {
	label=$1
	dir=$2
	forbidden=$3
	exclude=${4:-}
	if [ ! -d "$dir" ]; then
		echo "architecture rule targets a missing directory: $dir ($label)"
		status=1
		return 0
	fi
	report_matches "$label ($dir)" \
		"\"$module/internal/($forbidden)(/[^\"]*)?\"" "$exclude" "$dir"
}

# report_unlisted label pattern allowed-regexp dir...
# Files that match the pattern must be inside the allowed path expression.
report_unlisted() {
	label=$1
	pattern=$2
	allowed=$3
	shift 3
	files=$(production_files '' "$@")
	[ -n "$files" ] || return 0
	# shellcheck disable=SC2086
	matched=$(grep -lE -e "$pattern" $files || true)
	for file in $matched; do
		if ! printf '%s\n' "$file" | grep -Eq "$allowed"; then
			echo "architecture violation: $label"
			echo "$file"
			status=1
		fi
	done
}

if compat_files=$(find internal cmd -name 'compat.go' 2>/dev/null) &&
	[ -n "$compat_files" ]; then
	echo "architecture violation: transitional compatibility files remain"
	echo "$compat_files"
	status=1
fi

# Retired packages must not return.
for retired in \
	internal/controller internal/insight \
	internal/persistence internal/handler internal/monitor \
	internal/pvc internal/probe internal/networkgraph \
	internal/storagegraph internal/statuswatch internal/graphcontext \
	internal/resource internal/kubeletmetrics internal/controlplane \
	internal/delivery/util internal/problem internal/reason \
	internal/signal internal/knowledge internal/core internal/story \
	internal/notice internal/model internal/message internal/constant \
	internal/k8s internal/client internal/crdwatch internal/startup \
	internal/filter
do
	if [ -d "$retired" ]; then
		echo "architecture violation: retired package returned: $retired"
		status=1
	fi
	report_matches "import of retired package $retired" \
		"\"$module/$retired(/[^\"]*)?\"" '' internal cmd
done

# Layering from AGENTS.md. Dependencies flow downward:
# inventory -> detection -> rootcause -> incident -> notification/compose
# -> pipeline -> app. notification and storage are leaves.
above_inventory='detection|rootcause|incident|notification|pipeline|storage'
above_inventory="$above_inventory|scope|delivery|alert|app"
forbid_imports "inventory imports an upper layer" \
	internal/inventory "$above_inventory|health|rbac"
report_matches "inventory model imports Kubernetes" \
	'"k8s.io/(client-go|api|apimachinery)/|internal/(inventory/kube|kubeclient)' \
	'internal/inventory/kube/' internal/inventory

forbid_imports "detection imports an upper layer" \
	internal/detection \
	'rootcause|incident|notification|pipeline|storage|scope|delivery|alert|app'
forbid_imports "rootcause imports an upper layer" \
	internal/rootcause \
	'incident|notification|pipeline|storage|scope|delivery|alert|app'
forbid_imports "incident imports an upper layer" \
	internal/incident \
	'notification|pipeline|storage|scope|delivery|alert|app'
forbid_imports "compose imports an upper layer" \
	internal/notification/compose \
	'pipeline|storage|scope|delivery|alert|app'
forbid_imports "storage imports a domain layer" \
	internal/storage \
	'inventory|detection|rootcause|incident|notification|pipeline|delivery|alert|app'
forbid_imports "scope imports an upper layer" \
	internal/scope \
	'rootcause|incident|notification|pipeline|delivery|alert|app'
forbid_imports "pipeline imports delivery or composition" \
	internal/pipeline 'delivery|alert|app|health|config'
forbid_imports "rbac imports the domain layers" \
	internal/rbac \
	'detection|rootcause|incident|notification|pipeline|delivery|alert|app'

# Only the composition root wires the pipeline. replay is test tooling
# that drives an engine through a recorded log.
report_matches "package other than app imports pipeline" \
	"\"$module/internal/pipeline\"" 'internal/(app|pipeline|replay)/' \
	internal cmd

# replay sits above the pipeline and below nothing: it never reaches
# delivery, persistence, configuration or a cluster, and only tests
# import it.
forbid_imports "replay imports delivery, storage or cluster access" \
	internal/replay \
	'app|delivery|alert|health|config|storage|scope|audit|kubeclient|rbac|inventory/kube'
report_matches "production code imports replay" \
	"\"$module/internal/replay(/[^\"]*)?\"" '^internal/replay/' internal cmd

# Shared leaf packages stay below every domain and infrastructure package.
# notification/compose is a domain package with its own rule above.
leaf_forbidden='app|pipeline|incident|rootcause|detection|inventory|storage'
leaf_forbidden="$leaf_forbidden|notification/compose|scope|delivery|alert"
leaf_forbidden="$leaf_forbidden|config|kubeclient"
for leaf in notification format redact ratelimit detection/reasons
do
	forbid_imports "leaf package imports an upper layer" \
		"internal/$leaf" "$leaf_forbidden" '^internal/notification/compose/'
done

# Provider adapters build payloads and use the delivery transport. They must
# not depend on the domain packages, Kubernetes access, or the application
# root. They may import the leaf notification package.
domain='pipeline|incident|rootcause|detection|inventory|storage'
domain="$domain|notification/compose|scope"
forbid_imports "provider imports domain or infrastructure" \
	internal/alert \
	"app|$domain|kubeclient|config|health|audit"
report_matches "provider imports a Kubernetes client library" \
	'"k8s.io/(client-go|api|apimachinery)/' '' internal/alert internal/delivery
forbid_imports "delivery imports domain or composition" \
	internal/delivery \
	"app|$domain|kubeclient"

# SDK-backed adapters may translate their SDK's error type, but shared HTTP
# classification belongs to delivery/transport so retry semantics cannot drift.
report_matches "provider classifies HTTP status directly" \
	'event\.ClassifyHTTP\(|http\.NewRequest|\.Do\(' '' internal/alert

# Slack and Discord use injected SDK clients for provider-owned protocol
# details. These are the only production provider files allowed to import
# net/http; raw HTTP and generic status handling remain in delivery/transport.
report_unlisted "unapproved provider SDK HTTP import" '"net/http"' \
	'^internal/alert/(slack/transport|discord/discord)\.go$' internal/alert

report_matches "provider calls compatibility HTTP helper" \
	'(OptionalHTTPClient|PostContext|PostWithClientContext|ClientFrom|clients[[:space:]]+\.\.\.\*http\.Client|dependencies[[:space:]]+\.\.\.transport\.Dependencies|transport\.Post\()' \
	'' internal/alert
report_matches "provider constructs a transport sender during delivery" \
	'transport\.New\(' '' internal/alert
report_matches "provider retains application configuration" \
	'config\.(App|Config)' '' internal/alert

# Delivery owns provider generations; the provider contract has one shape.
report_matches "legacy provider adapter remains" \
	'LegacyProvider|ContextProvider|providerContextAdapter|adaptProvider|SendEventContext|SendMessageContext' \
	'' internal
report_matches "delivery retains pointer-based fallback state" \
	'fallback[[:space:]]+\*providerEntry' '' internal/delivery
report_matches "delivery keeps a duplicate provider slice" \
	'^[[:space:]]+entries[[:space:]]+\[\][[:space:]]*providerEntry' \
	'' internal/delivery
report_matches "delivery configuration setter remains" \
	'func \(.*\*Manager\) Set(Templates|Silences)\(' '' internal/delivery
report_matches "application discards component runner errors" \
	'componentRun\(' '' internal/app
report_matches "app uses the compatibility delivery initializer" \
	'InitWithFactory\(' '' internal/app
report_matches "health owns a context shutdown watcher" \
	'go[[:space:]]+.*(ctx|context).*Done|go[[:space:]]+.*Shutdown' \
	'' internal/health

# Dynamic informer construction belongs to dynamicwatch.
report_matches "dynamic informer construction outside dynamicwatch" \
	'k8s.io/client-go/dynamic/dynamicinformer' \
	'^internal/inventory/kube/dynamicwatch/' internal

# The YAML model is a loading boundary, not a runtime dependency.
report_unlisted "raw config.Config runtime consumer" \
	'config[.]Config' \
	'^(internal/(config|app)|cmd/(configcatalog|kwatch))/' \
	internal cmd

# Client construction is an application concern.
report_unlisted "local Kubernetes client construction" \
	'kubernetes\.NewForConfig|dynamic\.NewForConfig|discovery\.NewDiscoveryClientForConfig|rest\.RESTClientFor' \
	'^(internal/(kubeclient|app)|cmd/[a-z0-9-]+)/' internal cmd

# Time-based decisions read an injected clock. The clock package is the
# single production boundary that calls the standard library clock.
report_matches "production code calls time.Now directly" \
	'time\.Now\(' '^internal/clock/clock\.go$' internal cmd
report_matches "production code calls the global clock helper directly" \
	'(^|[^[:alnum:]_.])clock\.Now\(' '^internal/clock/clock\.go$' \
	internal cmd
report_matches "retired clock fallback remains" \
	'clock\.From\(' '^internal/clock/' internal cmd

report_matches "production code uses a process-wide HTTP client" \
	'http\.DefaultClient' '' internal cmd
report_matches "production code uses a process-wide DNS resolver" \
	'net\.DefaultResolver' '' internal cmd
report_matches "production code hides dependencies in context values" \
	'context\.WithValue\(' '' internal cmd

# Prometheus label values are bounded by construction; rejecting the dynamic
# API keeps names and error strings from becoming label cardinality.
report_matches "production code uses dynamic Prometheus label values" \
	'WithLabelValues\(' '' internal

exit "$status"
