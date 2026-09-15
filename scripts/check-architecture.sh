#!/bin/sh

# Enforce the dependency direction described in AGENTS.md. This is a small
# import-level guard, not a replacement for design review or Go compilation.
set -eu

root_dir=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root_dir"

status=0

provider_upper_layer_pattern=$(printf '%s%s' \
	'"github.com/abahmed/kwatch/internal/(app|controller|handler|' \
	'incident|persistence|k8s)"')

incident_forbidden_pattern=$(printf '%s' \
	'"github.com/abahmed/kwatch/internal/(audit|delivery|persistence)"')

insight_forbidden_pattern=$(printf '%s' \
	'"github.com/abahmed/kwatch/internal/(audit|delivery|persistence|k8s)"')

persistence_forbidden_pattern=$(printf '%s' \
	'"github.com/abahmed/kwatch/internal/(insight|delivery|handler|controller)"')

filter_forbidden_pattern=$(printf '%s' \
	'"github.com/abahmed/kwatch/internal/(app|controller|handler|' \
	'incident|insight|delivery|alert|persistence|startup|upgrader|k8s)"')

monitor_upper_layer_pattern=$(printf '%s%s%s' \
	'"github.com/abahmed/kwatch/internal/(app|controller|handler|' \
	'delivery|alert|persistence|startup|upgrader|k8s)"')

leaf_upper_layer_pattern=$(printf '%s%s%s' \
	'"github.com/abahmed/kwatch/internal/(app|controller|handler|' \
	'incident|alert|delivery|k8s|persistence|' \
	'startup|upgrader)"')

integration_incident_pattern='"github.com/abahmed/kwatch/internal/incident"'

delivery_provider_import_pattern='"github.com/abahmed/kwatch/internal/alert/'

report_matches() {
	label=$1
	pattern=$2
	shift 2
	if matches=$(rg -n "$pattern" "$@" 2>/dev/null); then
		echo "architecture violation: $label"
		echo "$matches"
		status=1
	fi
}

# Provider adapters build payloads and use the delivery transport. They must
# not depend on orchestration, Kubernetes access, or the application root.
report_matches \
	"provider package imports an upper-layer package" \
	"$provider_upper_layer_pattern" \
	internal/alert \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!util/**'

# SDK-backed adapters may translate their SDK's error type, but shared HTTP
# classification belongs to delivery/transport so retry semantics cannot drift.
report_matches \
	"provider package classifies HTTP status directly" \
	'event\.ClassifyHTTP\(|http\.NewRequest|\.Do\(' \
	internal/alert \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!util/**'

# Slack and Discord use injected SDK clients for provider-owned protocol
# details. These are the only production provider files allowed to import
# net/http; raw HTTP and generic status handling remain in delivery/transport.
provider_http_imports=$(rg -l '"net/http"' internal/alert \
	--glob '*.go' --glob '!**/*_test.go' || true)
for provider_file in $provider_http_imports; do
	case "$provider_file" in
		internal/alert/slack/transport.go|internal/alert/discord/discord.go)
			;;
		*)
			echo "architecture violation: unapproved provider SDK HTTP import"
			echo "$provider_file"
			status=1
			;;
	esac
done

report_matches \
	"provider package calls compatibility HTTP helper" \
	'(OptionalHTTPClient|PostContext|PostWithClientContext|ClientFrom|clients[[:space:]]+\.\.\.\*http\.Client|dependencies[[:space:]]+\.\.\.transport\.Dependencies|transport\.Post\()' \
	internal/alert \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"provider constructs a transport sender during delivery" \
	'transport\.New\(' \
	internal/alert \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"provider package imports retired delivery util package" \
	'"github.com/abahmed/kwatch/internal/delivery/util"' \
	internal/alert \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"provider retains application configuration" \
	'config\\.(App|Config)|\\*config\\.(App|Config)' \
	internal/alert \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Provider migration is complete: the delivery boundary must not grow a
# second provider contract or a context adapter. Context belongs in the
# provider method and is passed to the shared transport sender.
report_matches \
	"legacy provider adapter remains" \
	'LegacyProvider|ContextProvider|providerContextAdapter|adaptProvider|SendEventContext|SendMessageContext' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"delivery retains pointer-based fallback state" \
	'fallback[[:space:]]+\*providerEntry' \
	internal/delivery \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"delivery keeps a duplicate provider slice" \
	'^[[:space:]]+entries[[:space:]]+\[\][[:space:]]*providerEntry' \
	internal/delivery \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Provider construction belongs to the application-selected static catalog.
# Delivery must remain usable with a test or future catalog without importing
# every provider implementation.
report_matches \
	"delivery package imports concrete provider adapters" \
	"$delivery_provider_import_pattern" \
	internal/delivery \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Monitor families detect facts and produce observations. They do not own
# application composition, delivery, provider adapters, or persistence.
report_matches \
	"monitor package imports an upper-layer package" \
	"$monitor_upper_layer_pattern" \
	internal/monitor \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"production code imports transitional handler package" \
	'"github.com/abahmed/kwatch/internal/handler"' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"retired aggregate monitor capability remains" \
	'components\.(Workloads|Runtime)' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Incident and insight contain domain decisions. Concrete infrastructure must
# be supplied by the application through small interfaces.
report_matches \
	"incident package imports concrete infrastructure" \
	"$incident_forbidden_pattern" \
	internal/incident \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"insight package imports an infrastructure package" \
	"$insight_forbidden_pattern" \
	internal/insight \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"persistence package imports an upper-layer package" \
	"$persistence_forbidden_pattern" \
	internal/persistence \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"filter package imports orchestration or infrastructure" \
	"$filter_forbidden_pattern" \
	internal/filter \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Runtime integrations report observations through narrow monitor contracts.
# They must not reach into incident lifecycle implementation directly.
for integration_dir in \
	internal/kubeletmetrics \
	internal/metricsapi \
	internal/probe \
	internal/pvc \
	internal/resource \
	internal/rbac \
	internal/controlplane \
	internal/statuswatch
do
	report_matches \
		"integration imports concrete incident lifecycle ($integration_dir)" \
		"$integration_incident_pattern" \
		"$integration_dir" \
		--glob '*.go' \
		--glob '!**/*_test.go'
done

# Family ownership is deliberately explicit. Cluster-resource policy and
# admission endpoint policy must not regain their former cross-family imports.
report_matches \
	"cluster monitor imports security monitor policy" \
	'"github.com/abahmed/kwatch/internal/monitor/security"' \
	internal/monitor/cluster \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"security monitor imports network monitor policy" \
	'"github.com/abahmed/kwatch/internal/monitor/network"' \
	internal/monitor/security \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"PVC monitor imports persistence implementation" \
	'"github.com/abahmed/kwatch/internal/persistence"' \
	internal/pvc \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"retired monitor runtime aggregate remains" \
	'monitorruntime\\.(RuntimeSet|Components)|runtime\\.(RuntimeSet|Components)|type[[:space:]]+Components[[:space:]]+struct' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go'

report_matches \
	"provider falls back to process-wide HTTP client" \
	'http[.]DefaultClient' \
	internal/alert \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Time and shutdown ownership are explicit. The real clock is created only in
# the clock package; health compatibility is the only allowed convenience
# goroutine because application composition uses Open/Serve/Stop.
report_matches \
	"production code reads the global wall clock" \
	'(time[.]Now[(]|clock[.]Now[(])' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!**/clock/clock.go'

report_matches \
	"health owns a shutdown goroutine" \
	'go[[:space:]]+func' \
	internal/health \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!compat.go'

report_matches \
	"incident source setter used outside compatibility" \
	'SetAttributionSources' \
	internal/incident \
	--glob '*.go' \
	--glob '!compat.go' \
	--glob '!**/*_test.go'

report_matches \
	"TLS source setter used outside compatibility" \
	'SetSecretLister' \
	internal/monitor/security \
	--glob '*.go' \
	--glob '!compat.go' \
	--glob '!**/*_test.go'

report_matches \
	"runtime integration uses the process-wide DNS resolver" \
	'net[.]DefaultResolver' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!internal/app/**'

# Raw YAML configuration is an input concern. Domain packages consume the
# compiled runtime snapshot so their dependencies remain explicit and stable.
report_matches \
	"domain package consumes raw YAML configuration" \
	'\*config[.]Config|\bconfig[.]Config[[:space:]]*\{' \
	internal/monitor internal/incident internal/insight internal/delivery \
	internal/persistence internal/pvc internal/controller internal/health \
	internal/probe internal/controlplane internal/statuswatch \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!**/compat.go'

# Domain decisions must use an injected clock. The clock package is the only
# production boundary allowed to read wall time directly.
report_matches \
	"production code reads wall clock directly" \
	'time[.]Now[[:space:]]*[(]' \
	internal cmd \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!internal/clock/**'

# Generic dynamic informer construction belongs in dynamicwatch. This keeps
# cache-sync and optional-resource lifecycle behavior consistent.
report_matches \
	"dynamic informer constructed outside dynamicwatch" \
	'dynamicinformer[.]New[A-Za-z]*Informer' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!internal/k8s/dynamicwatch/**'

# Health owns its listener, while app owns shutdown and cancellation. A
# goroutine in the canonical health implementation would create a second
# lifecycle owner; compatibility shims are isolated in compat.go.
report_matches \
	"health package owns a shutdown goroutine" \
	'go[[:space:]]+func' \
	internal/health \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!**/compat.go'

# External clients are application-owned. A default client hides transport
# configuration and makes cancellation and tests unpredictable.
report_matches \
	"production code uses the process-wide HTTP client" \
	'http[.]DefaultClient' \
	internal cmd \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Dynamic informer construction belongs to dynamicwatch. Domain packages may
# use its shared factory, but must not create a second lifecycle implementation.
report_matches \
	"dynamic informer construction outside dynamicwatch" \
	'k8s.io/client-go/dynamic/dynamicinformer' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!internal/k8s/dynamicwatch/**' \
	--glob '!**/compat.go'

report_matches \
	"dynamic client construction outside compatibility code" \
	'(dynamic\\.NewForConfig|discovery\\.NewDiscoveryClientForConfig|rest\\.RESTClientFor)' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!**/compat.go' \
	--glob '!internal/client/**'

report_matches \
	"direct group-key parsing outside the codec" \
	'strings\\.Split\\([^)]*"\\|"' \
	internal/incident \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!group_key.go'

report_matches \
	"production code uses the wall clock outside the clock package" \
	'(time\\.Now\\(\\)|clock\\.Now\\(\\))' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!internal/clock/**'

report_matches \
	"production code uses the compatibility delivery initializer" \
	'InitWithFactory\(' \
	internal/app \
	--glob '*.go' \
	--glob '!**/*_test.go'

# The YAML model is a loading boundary, not a runtime dependency. These are
# the only production locations allowed to retain it: decoding, application
# bootstrap, CRD overlays, migrations, command metadata, and compatibility.
raw_config_files=$(rg -l \
	'config[.]Config|[*]config[.]Config' \
	internal cmd \
	--glob '*.go' \
	--glob '!**/*_test.go' || true)
for raw_config_file in $raw_config_files; do
	case "$raw_config_file" in
		internal/config/*|internal/app/*|internal/crdwatch/*|\
		internal/controller/compat.go|internal/delivery/compat.go|\
		internal/rbac/compat.go|internal/client/compat.go|\
		cmd/configcatalog/*)
			;;
		*)
			echo "architecture violation: raw config.Config runtime consumer"
			echo "$raw_config_file"
			status=1
			;;
	esac
done

report_matches \
	"incident package imports concrete Kubernetes listers" \
	'k8s.io/client-go/listers/' \
	internal/incident \
	--glob '*.go' \
	--glob '!**/*_test.go'

# REST and dynamic client construction is an application concern. Explicitly
# named compatibility files may retain the old constructors for embedded
# callers while production composition uses internal/client.ClientSet.
for construction_dir in \
	internal/networkgraph \
	internal/storagegraph \
	internal/statuswatch \
	internal/metricsapi \
	internal/controlplane \
	internal/crdwatch
do
	report_matches \
		"client construction outside compatibility boundary ($construction_dir)" \
		'(dynamic\.NewForConfig|discovery\.NewDiscoveryClientForConfig|rest\.RESTClientFor)' \
		"$construction_dir" \
		--glob '*.go' \
		--glob '!**/*_test.go' \
		--glob '!**/compat.go'
done

# Optional dynamic monitors share informer construction. CRD-specific restart
# policy remains in crdwatch, but it must not fork the low-level mechanics.
report_matches \
	"dynamic monitor constructs informers outside dynamicwatch" \
	'"k8s.io/client-go/dynamic/dynamicinformer"' \
	internal/statuswatch \
	internal/crdwatch \
	internal/networkgraph \
	internal/storagegraph \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Shared value and graph packages stay below orchestration and integrations.
for package_dir in \
	internal/model \
	internal/event \
	internal/graphcontext \
	internal/constant \
	internal/format
do
	report_matches \
		"leaf package imports an upper-layer package ($package_dir)" \
		"$leaf_upper_layer_pattern" \
		"$package_dir" \
		--glob '*.go' \
		--glob '!**/*_test.go'
done

# Decisions must use the injected clock seam. The clock package is the single
# production boundary allowed to call the standard library clock directly.
report_matches \
	"production code calls time.Now directly" \
	'time\.Now\(' \
	internal cmd \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!internal/clock/clock.go'

report_matches \
	"production code calls the global clock helper directly" \
	'clock\.Now\(' \
	internal cmd \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!internal/clock/clock.go'

report_matches \
	"production code uses the compatibility incident constructor" \
	'incident\.NewEngine\(' \
	internal cmd \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!**/compat.go'

report_matches \
	"production code uses the compatibility clock fallback" \
	'clock\.From\(' \
	internal cmd \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!**/compat.go' \
	--glob '!internal/clock/**'

report_matches \
	"feedback clock can be mutated after construction" \
	'func \(.*\*FeedbackStore\) SetClock\(' \
	internal/insight \
	--glob '*.go' \
	--glob '!**/*_test.go'

# Prometheus label values are intentionally absent from current internal
# metrics. Rejecting the dynamic API keeps resource names and error strings
# from becoming unbounded cardinality if metrics are extended later.
report_matches \
	"production code uses dynamic Prometheus label values" \
	'WithLabelValues\(' \
	internal \
	--glob '*.go' \
	--glob '!**/*_test.go'

exit "$status"
