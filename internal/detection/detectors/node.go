package detectors

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultNodeRemoval is how long node deletion may take before it is
// reported as stuck.
const DefaultNodeRemoval = 30 * time.Minute

// DefaultNodeNotReady is how long a node may be NotReady before it is a
// finding; brief kubelet restarts recover faster than this.
const DefaultNodeNotReady = 90 * time.Second

// nodePressure maps pressure conditions to their finding reasons.
var nodePressure = []struct {
	condition string
	reason    string
	summary   string
}{
	{"MemoryPressure", reasons.MemoryPressure,
		"Node is low on memory; pods may be evicted"},
	{"DiskPressure", reasons.DiskPressure,
		"Node is low on disk space; pods may be evicted"},
	{"PIDPressure", reasons.PIDPressure,
		"Node is running out of process IDs"},
}

// Node detects NotReady or unreachable nodes, resource pressure and
// network unavailability.
type Node struct {
	notReady time.Duration
}

// NewNode builds the node detector.
func NewNode(notReady time.Duration) Node {
	if notReady <= 0 {
		notReady = DefaultNodeNotReady
	}
	return Node{notReady: notReady}
}

// Name implements detection.Detector.
func (Node) Name() string { return "node" }

// Kinds implements detection.Detector.
func (Node) Kinds() []inventory.Kind { return []inventory.Kind{kube.KindNode} }

// Detect implements detection.Detector.
func (d Node) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	draining, drain := drainFinding(e)
	if draining {
		out = append(out, d.drainOrStuck(ctx, e, drain))
	}
	if s, ok := d.readiness(ctx, e); ok &&
		!expectedNotReady(ctx, drain, s.Since) {
		out = append(out, s)
	}
	if s, ok := heartbeatFinding(ctx, e); ok &&
		!expectedNotReady(ctx, drain, s.Since) {
		out = append(out, s)
	}
	out = append(out, sandboxFindings(ctx, e)...)
	for _, p := range nodePressure {
		status, _, since := condition(e, p.condition)
		if status == "True" {
			out = append(out, detection.Finding{
				Reason: p.reason, Severity: detection.Critical,
				Since: since, Summary: p.summary,
			})
		}
	}
	if status, reason, since := condition(e, "NetworkUnavailable"); status ==
		"True" {
		out = append(out, detection.Finding{
			Reason:   reasons.NetworkUnavailable,
			Severity: detection.Critical, Since: since,
			Summary: "Node network is not configured (" + reason + ")",
		})
	}
	return out
}

// drainEnvelope bounds how long a cordoned or departing node's NotReady is
// treated as part of the drain. Maintenance reboots and scale-down
// shutdowns go NotReady on purpose; a node that stays NotReady past the
// envelope is broken, not draining, and is reported again.
const drainEnvelope = DefaultNodeRemoval

// expectedNotReady reports a NotReady that belongs to a drain still inside
// its envelope. A node that was NotReady before it was cordoned is broken
// regardless of the drain. Pressure and network conditions are never
// hidden: a drain does not explain them, and they may be why the node is
// drained.
func expectedNotReady(
	ctx detection.Context, drain detection.Finding, notReadySince time.Time,
) bool {
	if drain.Reason == "" {
		return false
	}
	if drain.Since.IsZero() {
		return true
	}
	if !notReadySince.IsZero() && notReadySince.Before(drain.Since) {
		return false
	}
	remaining := drainEnvelope - ctx.Now.Sub(drain.Since)
	if remaining <= 0 {
		return false
	}
	ctx.RecheckAfter(remaining)
	return true
}

// drainOrStuck returns the drain finding, or NodeStuckTerminating when a
// deleting node never finishes leaving.
func (Node) drainOrStuck(
	ctx detection.Context, e inventory.Entity, s detection.Finding,
) detection.Finding {
	if !flag(e, kube.AttrDeleting) {
		return s
	}
	if ctx.Now.Sub(s.Since) < DefaultNodeRemoval {
		ctx.RecheckAfter(s.Since.Add(DefaultNodeRemoval).Sub(ctx.Now))
		return s
	}
	s.Reason, s.Severity = reasons.NodeStuckTerminating,
		detection.Warning
	s.Summary = "Node has been deleting for " +
		format.Duration(ctx.Now.Sub(s.Since)) +
		"; a finalizer or cloud controller may be stuck"
	return s
}

func (d Node) readiness(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	status, reason, since := condition(e, "Ready")
	if status == "" || status == "True" {
		return detection.Finding{}, false
	}
	if !sustained(ctx, "node-not-ready", since, d.notReady) {
		return detection.Finding{}, false
	}
	summary := "Node is NotReady for " + format.Duration(ctx.Now.Sub(since))
	if status == "Unknown" {
		summary = "Node stopped reporting (kubelet unreachable) for " +
			format.Duration(ctx.Now.Sub(since))
	}
	return detection.Finding{
		Reason: reasons.NodeNotReady, Severity: detection.Critical,
		Since: since, Summary: summary,
		Evidence: []detection.Evidence{
			{Label: "reason", Value: reason},
			{Label: "message", Value: conditionMessage(e, "Ready")},
		},
	}, true
}

// drainFinding reports a node that is cordoned or being deleted: a drain,
// an upgrade, autoscaler scale-down or spot replacement.
func drainFinding(e inventory.Entity) (bool, detection.Finding) {
	deleting := flag(e, kube.AttrDeleting)
	removal := removalTaint(text(e, kube.AttrTaints))
	if !deleting && !flag(e, kube.AttrUnschedulable) && removal == "" {
		return false, detection.Finding{}
	}
	summary, since := "Node is cordoned for maintenance",
		valueSince(e, kube.AttrUnschedulable)
	switch {
	case deleting:
		summary, since = "Node is being removed",
			valueSince(e, kube.AttrDeleting)
	case removal != "":
		summary, since = "Node is being removed ("+removal+")",
			valueSince(e, kube.AttrTaints)
	}
	return true, detection.Finding{
		Reason: reasons.NodeDraining, Severity: detection.Info,
		Since: since, Summary: summary,
	}
}

// removalTaints are the taints Kubernetes and its autoscalers put on a
// node they are taking away, each with how a message names the reason.
var removalTaints = []struct{ key, reason string }{
	{"node.kubernetes.io/out-of-service", "marked out of service"},
	{"ToBeDeletedByClusterAutoscaler", "scale-down"},
	{"DeletionCandidateOfClusterAutoscaler", "scale-down"},
	{"node.kubernetes.io/unschedulable", "cordoned"},
}

// removalTaint names the removal a node's taints announce, or "".
func removalTaint(taints string) string {
	for _, taint := range removalTaints {
		if strings.Contains(taints, taint.key+":") ||
			strings.Contains(taints, taint.key+"=") {
			return taint.reason
		}
	}
	return ""
}
