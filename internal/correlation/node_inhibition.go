package correlation

import (
	"sort"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// trackNodeIncident records an active node incident for pod suppression. It
// must run before the baseline check so node events always populate the
// inhibition map. Synthetic capacity signals (resource overcommit) must not
// suppress pods. Caller must hold e.mu.
func (e *Engine) trackNodeIncident(res, nodeName, reason string) {
	if res == "node" && nodeName != "" && nodeIncidentInhibitsPods(reason) {
		e.activeNodeIncidents[nodeName] = true
	}
}

// recordSuppressedPod bumps the suppression counters on a node incident.
func recordSuppressedPod(
	nodeInc *model.Incident,
	ev event.Event,
	owner string,
) {
	nodeInc.SuppressedPods++
	if owner != "" {
		if nodeInc.SuppressedOwners == nil {
			nodeInc.SuppressedOwners = make(map[string]int)
		}
		nodeInc.SuppressedOwners[owner]++
	}
	if len(nodeInc.SuppressedPodSummaries) < 20 {
		nodeInc.SuppressedPodSummaries = append(
			nodeInc.SuppressedPodSummaries,
			model.PodSummary{
				Namespace:    ev.Namespace,
				PodName:      ev.PodName,
				Reason:       ev.Reason,
				RestartCount: ev.RestartCount,
			},
		)
	}
}

// Caller must hold e.mu.
func (e *Engine) findNodeIncident(nodeName string) *model.Incident {
	keys := make([]string, 0, len(e.state))
	for key := range e.state {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		inc := e.state[model.IncidentKey(rawKey)]
		if inc.Resource == "node" && inc.Name == nodeName {
			return inc
		}
	}
	return nil
}

// findMostConstrainedNodeIncident returns the node incident with the most
// suppressed pods, used as a target for unschedulable-pod suppression.
// PendingResolve incidents (recovered conditions) are not targets.
// Caller must hold e.mu.
func (e *Engine) findMostConstrainedNodeIncident() *model.Incident {
	var best *model.Incident
	keys := make([]string, 0, len(e.state))
	for key := range e.state {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		inc := e.state[model.IncidentKey(rawKey)]
		if inc.Resource == "node" && inc.State != model.StateResolved &&
			inc.State != model.StatePendingResolve {
			if best == nil || inc.SuppressedPods > best.SuppressedPods ||
				(inc.SuppressedPods == best.SuppressedPods &&
					string(inc.Key) < string(best.Key)) {
				best = inc
			}
		}
	}
	return best
}

// CountActiveNodeIncidents returns the number of nodes with active
// (non-resolved) incidents. Used for node→resource inhibition decisions.
func (e *Engine) CountActiveNodeIncidents() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.activeNodeIncidents)
}

// SetActiveNodeIncidents marks the given nodes as having active incidents.
// Used at startup to pre-populate inhibition before any worker runs.
func (e *Engine) SetActiveNodeIncidents(nodeNames []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range nodeNames {
		e.activeNodeIncidents[n] = true
	}
}

// refreshNodeInhibition clears the node inhibition flag if no active node
// incidents remain for this node. A StatePendingResolve incident does not
// inhibit pods: its condition has already recovered and the hold-down only
// delays the "resolved" notification. Caller must hold e.mu.
func (e *Engine) refreshNodeInhibition(nodeName string) {
	for _, inc := range e.state {
		if inc.Resource == "node" && inc.Name == nodeName &&
			inc.State != model.StateResolved &&
			inc.State != model.StatePendingResolve {
			return
		}
	}
	delete(e.activeNodeIncidents, nodeName)
}

// RefreshNodeInhibition recomputes the node suppression flag after a node
// condition resolves. Unlike MarkResolved it is safe to call for baselined
// nodes that never had an incident, so stale suppression is cleared as soon
// as the node recovers.
func (e *Engine) RefreshNodeInhibition(nodeName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.refreshNodeInhibition(nodeName)
}

// nodeLevelReasons are incident reasons that indicate a node-level problem.
// Pod-level reasons (Error, CrashLoopBackOff, etc.) should NOT be scoped
// as "node" even when all entries share the same node, because the message
// would misleadingly imply the node is the root cause.
var nodeLevelReasons = map[string]bool{
	constant.ReasonNodeNotReady:         true,
	constant.ReasonMemoryPressure:       true,
	constant.ReasonDiskPressure:         true,
	constant.ReasonPIDPressure:          true,
	constant.ReasonNetworkUnavailable:   true,
	constant.ReasonNodeResourceHigh:     true,
	constant.ReasonNodeResourceCritical: true,
	constant.ReasonContainerStatusKnown: true,
	constant.ReasonEvicted:              true,
	constant.ReasonPreempting:           true,
}

func isNodeLevelReason(r string) bool {
	return nodeLevelReasons[r]
}

// nodeIncidentInhibitsPods reports whether an active incident for the given
// node-level reason should suppress pod alerts. Synthetic capacity signals
// (resource overcommit) must not inhibit pods.
func nodeIncidentInhibitsPods(r string) bool {
	switch r {
	case constant.ReasonNodeResourceHigh, constant.ReasonNodeResourceCritical:
		return false
	}
	return nodeLevelReasons[r]
}
