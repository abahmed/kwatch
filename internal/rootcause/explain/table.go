package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// LinkType names how a cause reaches an effect. The names read from the
// effect's point of view ("the pod runs-on the node") or the cause's
// ("the deployment owns the pod"), as in ADR 0011.
type LinkType string

// Link types of the propagation table.
const (
	// LinkRunsOn: the effect runs on the cause (pod on node).
	LinkRunsOn LinkType = "runs-on"
	// LinkOwns: the cause owns the effect (deployment owns pods).
	LinkOwns LinkType = "owns"
	// LinkUses: the effect uses the cause by name (pod uses secret).
	LinkUses LinkType = "uses"
	// LinkMounts: the effect mounts the cause (pod mounts claim).
	LinkMounts LinkType = "mounts"
	// LinkPulls: the effect pulls images from the cause (registry).
	LinkPulls LinkType = "pulls"
	// LinkManages: the cause is the controller that writes the effect.
	LinkManages LinkType = "manages"
	// LinkAdmits: the cause admits the effect's creates (webhook,
	// quota).
	LinkAdmits LinkType = "admits"
	// LinkServedBy: the effect is served by the cause (a webhook by
	// its Service, an HPA by the metrics APIService).
	LinkServedBy LinkType = "served-by"
	// LinkContains: the cause contains the effect (namespace, zone).
	LinkContains LinkType = "contains"
	// LinkResolvesVia: the effect resolves names via the cause.
	LinkResolvesVia LinkType = "resolves-via"
	// LinkSchedules: the cause is the scheduling constraint that keeps
	// the effect pending.
	LinkSchedules LinkType = "schedules"
	// LinkRestricts: the cause is a policy that selects the effect.
	LinkRestricts LinkType = "restricts"
	// LinkBacks: the cause backs the effect (pods behind a Service).
	LinkBacks LinkType = "backs"
	// LinkRoutesTo: the effect routes traffic to the cause (an Ingress
	// or a route to its backend Service).
	LinkRoutesTo LinkType = "routes-to"
)

// AnyKind and AnyGroup make a side match every kind or every group.
const (
	AnyKind  inventory.Kind = "*"
	AnyGroup                = "*"
)

// Pseudo modes describe candidates that have no finding of their own.
// They are matched like finding modes.
const (
	// ModeChanged is any meaningful change inside the causal window.
	ModeChanged detection.Mode = "Changed"
	// ModeSpecChanged is a change of anything but the replica count:
	// a rollout of a new template.
	ModeSpecChanged detection.Mode = "Changed.Spec"
	// ModeScaled is a change of only the replica count.
	ModeScaled detection.Mode = "Changed.Scale"
	// ModeCreated is the creation of the object.
	ModeCreated detection.Mode = "Changed.Created"
	// ModeMissing is a referenced object that does not exist.
	ModeMissing detection.Mode = "Missing"
	// ModeMembersFailing is a zone or pool with failing nodes.
	ModeMembersFailing detection.Mode = "MembersFailing"
	// ModeRejectsNodes is a scheduling constraint the scheduler named.
	ModeRejectsNodes detection.Mode = "RejectsNodes"
	// ModeResolution is name resolution failing, seen from clients.
	ModeResolution detection.Mode = "Resolution"
	// ModeMetricsUnserved is a metrics API that cannot answer, seen
	// from the autoscalers that read it.
	ModeMetricsUnserved detection.Mode = "MetricsUnserved"
)

// Side selects the cause or the effect of a row. A mode matches itself
// and every finer mode below it: "ImagePull" matches
// "ImagePull.Registry".
type Side struct {
	// Group is the API group; "" is the built-in group, AnyGroup is
	// every group.
	Group string
	// Kind is the entity kind, or AnyKind. Pod rows also match the
	// pod's containers.
	Kind inventory.Kind
	// Modes lists the modes that match. Empty matches any finding
	// mode but no pseudo mode: a change or an absence must be named.
	Modes []detection.Mode
	// NotModes lists modes that never match, even if Modes does.
	NotModes []detection.Mode
	// Health lists the health states that match; empty matches
	// Failing and Degraded. Pseudo modes count as Failing.
	Health []detection.Health
	// Signal names a class of error text the effect must show, such
	// as a DNS error. Empty requires nothing.
	Signal Signal
	// Custom limits the side to custom resources: an entity of an API
	// group kube-apiserver does not serve itself.
	Custom bool
}

// Row says that a cause in some modes, through one link, explains
// effects in some modes, with a prior belief before any evidence.
type Row struct {
	// Name identifies the row in traces and tests.
	Name   string
	Cause  Side
	Link   LinkType
	Effect Side
	// Prior is the belief before evidence, between 0 and 1.
	Prior float64
	// MinCovered is the fewest effects the cause must explain for the
	// row to apply. A zone with one failing node is that node's
	// problem, not the zone's.
	MinCovered int
	// MinWorkloads is the fewest distinct workloads the effects must
	// belong to. A shared endpoint blamed by one app's replicas is
	// that app's problem; several apps failing on it is a pattern.
	MinWorkloads int
	// InferredMinWorkloads replaces MinWorkloads, when larger, for a
	// cause that shows only a pseudo mode: a shared service read from
	// one app's errors is that app's problem until others fail too.
	InferredMinWorkloads int
	// Inside marks a cause that is part of the effect's own workload:
	// its limits, its probes, one of its other containers, the custom
	// resource that owns it. Such a cause names no outside object, but
	// it is still a cause and the incident states it.
	Inside bool
}

// InsideRow reports whether the named row blames a part of the
// failure's own workload (see Row.Inside).
func InsideRow(name string) bool {
	for _, row := range propagation {
		if row.Name == name {
			return row.Inside
		}
	}
	return false
}

// summaryModes are workload findings that summarise their pods. They
// are consequences, never causes: a Deployment is Unavailable because
// its pods fail.
var summaryModes = []detection.Mode{
	detection.ModeUnavailable, detection.ModeRolloutStuck,
	detection.ModeStatefulSetRolloutStuck, detection.ModeConditionFailure,
	detection.ModeCondition, detection.ModeDisruptionBudget,
	detection.ModeNoEndpoints, detection.ModeBackendsDegraded}

// propagation is the propagation table: generic rows cover every
// kind, specific rows cover the most common failures precisely. The
// best matching row by prior wins for each cause and effect pair.
var propagation = concatRows(genericRows, specificRows)

// Table returns a copy of the propagation table, for documentation
// and tests.
func Table() []Row {
	return concatRows(propagation)
}
