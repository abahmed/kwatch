package detectors

import (
	"sort"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Node heartbeat staleness. The kubelet renews its Lease in
// kube-node-lease every 10s; when neither the Lease nor the node status
// is renewed for the node-monitor-grace-period, the node lifecycle
// controller sets Ready=Unknown with reason NodeStatusUnknown
// (pkg/controller/nodelifecycle). kwatch reads that condition instead of
// watching Lease specs: Leases are watched for metadata only, because
// their renewals would otherwise update the model every 10s per node.
const (
	nodeStatusUnknown = "NodeStatusUnknown"
	// nodeHeartbeatGrace is how long the heartbeat must stay stale after
	// the controller noticed, so a brief API hiccup is not reported.
	nodeHeartbeatGrace = 40 * time.Second
)

// heartbeatFinding reports a node whose kubelet stopped renewing its
// heartbeat. It comes earlier than NodeNotReady, which waits longer.
func heartbeatFinding(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	status, reason, since := condition(e, "Ready")
	if status != "Unknown" || reason != nodeStatusUnknown ||
		!sustained(ctx, "node-heartbeat", since, nodeHeartbeatGrace) {
		return detection.Finding{}, false
	}
	if holding, _ := joinHold(ctx, e, status, since); holding {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.NodeHeartbeatStale, Severity: detection.Warning,
		Since: since,
		Summary: "Node heartbeat is stale: the kubelet stopped renewing " +
			"its lease",
		Evidence: []detection.Evidence{{
			Label: "stale for", Value: format.Duration(ctx.Now.Sub(since)),
		}},
	}, true
}

// Pod network failures aggregate to the node when this many pods on it
// fail the same way within EventWindow.
const (
	nodeSandboxPods = 3
	// sandboxRecheck re-evaluates a node while pods on it are Pending:
	// their sandbox events touch only the pod, never the node.
	sandboxRecheck = time.Minute
)

// nodeSandboxReasons are the node findings for each failure class.
var nodeSandboxReasons = map[detection.Mode]struct {
	reason  string
	summary string
}{
	detection.ModeNetworkIPExhausted: {reasons.NodePodIPExhausted,
		"Node has run out of pod IP addresses"},
	detection.ModeNetworkCNINotReady: {reasons.NodeCNINotReady,
		"Node network plugin is not ready; pods cannot get a network"},
}

// sandboxFindings reports a node on which several pods cannot get their
// network for the same reason: IP exhaustion or an uninitialised CNI
// plugin is a node problem, not a pod one.
func sandboxFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil {
		return nil
	}
	t := sandboxTally{pods: map[detection.Mode]int{},
		stuck:  map[detection.Mode]int{},
		first:  map[detection.Mode]time.Time{},
		newest: map[detection.Mode]inventory.Note{}}
	anyPending := false
	for _, id := range ctx.Model.Related(
		e.ID, inventory.RunsOn, inventory.Incoming,
	) {
		pod, ok := ctx.Model.Entity(id)
		anyPending = anyPending ||
			ok && text(pod, kube.AttrPhase) == "Pending"
		// A pod that got its address since is ready, or done.
		stuck := ok && !podFinished(pod) && !flag(pod, kube.AttrReady)
		for mode, note := range podSandboxFailures(ctx, id) {
			t.add(mode, note, stuck)
		}
	}
	if anyPending {
		ctx.RecheckAfter(sandboxRecheck)
	}
	return buildSandboxFindings(ctx, t)
}

// sandboxTally counts, per failure class, the pods that failed within
// the event window and the ones of them that have not started since,
// with the first and the newest event.
type sandboxTally struct {
	pods, stuck map[detection.Mode]int
	first       map[detection.Mode]time.Time
	newest      map[detection.Mode]inventory.Note
}

func (t sandboxTally) add(
	mode detection.Mode, note inventory.Note, stuck bool,
) {
	t.pods[mode]++
	if stuck {
		t.stuck[mode]++
	}
	if t.first[mode].IsZero() || note.At.Before(t.first[mode]) {
		t.first[mode] = note.At
	}
	if note.At.After(t.newest[mode].At) {
		t.newest[mode] = note
	}
}

// podSandboxFailures returns the latest event of each classified network
// failure of one pod.
func podSandboxFailures(
	ctx detection.Context, pod inventory.EntityID,
) map[detection.Mode]inventory.Note {
	out := make(map[detection.Mode]inventory.Note)
	for _, note := range ctx.Model.Notes(pod, ctx.Now.Add(-EventWindow)) {
		if mode, ok := sandboxFailure(note); ok &&
			note.At.After(out[mode].At) {
			out[mode] = note
		}
	}
	return out
}

func buildSandboxFindings(
	ctx detection.Context, t sandboxTally,
) []detection.Finding {
	modes := make([]detection.Mode, 0, len(t.pods))
	for mode, count := range t.pods {
		if count >= nodeSandboxPods {
			modes = append(modes, mode)
		}
	}
	sort.Slice(modes, func(i, j int) bool { return modes[i] < modes[j] })
	out := make([]detection.Finding, 0, len(modes))
	for _, mode := range modes {
		spec := nodeSandboxReasons[mode]
		ctx.RecheckAfter(t.first[mode].Add(EventWindow).Sub(ctx.Now))
		out = append(out, detection.Finding{
			Reason: spec.reason, Severity: detection.Critical,
			Since: t.first[mode], Summary: spec.summary,
			Evidence: []detection.Evidence{{
				Label: "affected pods", Value: strconv.Itoa(t.pods[mode]),
			}, {
				Label: detection.EvidenceStuckPods,
				Value: strconv.Itoa(t.stuck[mode]),
			}, {
				Label: detection.EvidenceSandboxEvent,
				Value: sandboxQuote(t.newest[mode].Message),
			}},
		})
	}
	return out
}
