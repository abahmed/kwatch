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
	if status == "" {
		return detection.Finding{}, false
	}
	if status == "True" {
		holdBlip(ctx, since)
		return detection.Finding{}, false
	}
	// A NotReady that follows a brief Ready blip continues the earlier
	// one: it keeps its onset, so the wait is not restarted by a flap.
	since = ctx.Onset(notReadyOnset, since)
	if !sustained(ctx, "node-not-ready", since, d.notReady) {
		return detection.Finding{}, false
	}
	ctx.Onset(reportedOnset, ctx.Now)
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

// notReadyOnset names the remembered start of a NotReady episode.
const notReadyOnset = "node-not-ready"

// reportedOnset marks an episode that already became a finding. Only
// such an episode survives a Ready blip: NotReady stretches that each
// stay under the threshold never add up to a finding.
const reportedOnset = "node-not-ready/reported"

// readyBlip is how long a node may be Ready between two NotReady
// stretches and still count as one flapping episode. Nodes blip like
// this while pools scale and consolidate.
const readyBlip = 2 * time.Minute

// holdBlip keeps the onset of a reported NotReady episode alive while the node
// has been Ready for less than readyBlip. Nothing is reported for the
// Ready node; only the onset survives, so a node that goes NotReady
// again resumes the old episode instead of starting a new one. After
// readyBlip of steady Ready the onset is forgotten.
func holdBlip(ctx detection.Context, readySince time.Time) {
	if !ctx.Ongoing(reportedOnset) {
		return
	}
	ready := ctx.Onset("node-ready-blip", readySince)
	remaining := readyBlip - ctx.Now.Sub(ready)
	if remaining <= 0 {
		return
	}
	ctx.Onset(notReadyOnset, ctx.Now)
	ctx.Onset(reportedOnset, ctx.Now)
	ctx.RecheckAfter(remaining)
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
// The unschedulable taint is not listed: every cordon adds it, so it
// says nothing beyond spec.unschedulable.
var removalTaints = []struct{ key, reason string }{
	{"node.kubernetes.io/out-of-service", "marked out of service"},
	{"ToBeDeletedByClusterAutoscaler", "scale-down"},
	{"DeletionCandidateOfClusterAutoscaler", "scale-down"},
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
