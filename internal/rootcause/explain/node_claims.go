package explain

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A node explains the failures of the pods on it only when the node is
// what changed for them. A pod that was failing before the node's
// problem began, a workload that fails the same way on nodes that are
// fine, and a container whose own output names its fault (a binary
// built for another CPU, an exception at start, a configuration it
// rejects or lacks) failed for its own reason: the node only hosts it.
// Without these checks a node under brief pressure, or one being
// drained, absorbs every crash loop that happens to run on it and
// borrows their severity.

// selfEvidentModes are failure modes whose finding names the
// container's own fault.
var selfEvidentModes = []detection.Mode{
	detection.ModeCreateErrorConfig, detection.ModeExitNotExecutable,
	detection.ModeMissingSecret, detection.ModeMissingConfigMap,
	detection.ModeMissingServiceAccount,
}

// nodeWrittenLine is a fatal line a node fault writes into a program:
// a file system the kernel turned read-only.
var nodeWrittenLine = regexp.MustCompile(`(?i)read-only file system`)

// weakNodeRows are the node rows that blame a node still running its
// pods (a struggling node, a lost network) or one taken away: pods on
// such a node fail for many other reasons, so the checks apply. A node
// that is down is judged by the temporal scorer instead.
var weakNodeRows = map[string]bool{
	"node-degraded": true, "node-removed": true, "node-network": true,
}

// applyNodeClaims drops the effects a node candidate reaches through
// the pods on it, by a weak node row, but did not cause (nodeDisowns).
func (v *view) applyNodeClaims(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		if id.Kind != kube.KindNode {
			continue
		}
		for _, effect := range sortedKeys(c.covers) {
			how := c.covers[effect]
			if how.derived || !weakNodeRows[how.match.row.Name] {
				continue
			}
			if reason := v.nodeDisowns(id, effect); reason != "" {
				c.dropCover(effect)
				cs.reject(id, effect, reason)
			}
		}
		if len(c.covers) == 0 {
			delete(cs.byID, id)
		}
	}
}

// nodeDisowns says why node did not cause effect, or "" when it may
// have.
func (v *view) nodeDisowns(node, effect inventory.EntityID) string {
	pod, ok := v.podOf(effect)
	if !ok {
		return ""
	}
	// A DaemonSet's pod serves its node (a network or storage agent):
	// its crash is the node's problem, often before the node shows it.
	agent := rootcause.TopOwner(v.s.Model, pod).Kind == kube.KindDaemonSet
	switch {
	case !agent && v.failedBeforeNode(node, pod):
		return "its pods failed before the node's problem began"
	case !agent && v.selfEvident(pod):
		return "its container's own output names its fault"
	case v.failsOnHealthyNode(node, pod):
		return "the same workload fails the same way on a healthy node"
	}
	return ""
}

// failedBeforeNode reports a pod that was failing before the node's own
// problem began: its earliest finding, or the time it last became
// unready, is older than the node's first unhealthy finding or its
// removal. A node start stamped when kwatch began, against a pod dated
// earlier by the cluster, mixes two clocks and proves nothing.
func (v *view) failedBeforeNode(node, pod inventory.EntityID) bool {
	began := v.nodeProblemStart(node)
	unit := v.unitStart(pod)
	if !began.ok || !unit.ok {
		return false
	}
	if v.observedAtStart(began.t) && v.predatesStart(unit.t) {
		return false
	}
	return unit.t.Before(began.t.Add(-TemporalSlack))
}

// nodeProblemStart is when the node went wrong: its earliest unhealthy
// finding or its deletion. Its creation and label edits are not a
// problem.
func (v *view) nodeProblemStart(node inventory.EntityID) timeValue {
	start := v.earliestSince([]inventory.EntityID{node})
	for _, change := range v.changesOf(node) {
		if change.Deleted {
			start = start.earlier(change.At)
		}
	}
	return start
}

// unitStart is when a pod began to fail: the earliest finding of it or
// its containers, or, for a pod that is not ready, when it last became
// unready.
func (v *view) unitStart(pod inventory.EntityID) timeValue {
	ids := append([]inventory.EntityID{pod}, v.s.Model.Related(pod,
		inventory.PartOf, inventory.Incoming)...)
	start := v.earliestSince(ids)
	e, ok := v.s.Model.Entity(pod)
	if !ok || attrBool(e, kube.AttrReady) {
		return start
	}
	if since, ok := attrTime(e, kube.AttrReadySince); ok &&
		!since.IsZero() {
		start = start.earlier(since)
	}
	return start
}

// selfEvident reports a pod one of whose containers names its own
// fault: a fatal line at start (not one a node writes), or a finding
// of a configuration it rejects or lacks, or a binary it cannot run.
func (v *view) selfEvident(pod inventory.EntityID) bool {
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		e, ok := v.s.Model.Entity(id)
		if !ok {
			continue
		}
		line := v.startupErrorLine(id, e)
		if line != "" && !nodeWrittenLine.MatchString(line) {
			return true
		}
		if v.hasMode(id, selfEvidentModes) || archEvidence(v.s.Findings[id]) {
			return true
		}
	}
	return v.hasMode(pod, selfEvidentModes)
}

// archEvidence reports a finding that says the container's binary is
// built for another CPU than its node's.
func archEvidence(findings []detection.Finding) bool {
	for _, f := range findings {
		for _, e := range f.Evidence {
			if e.Label == detection.EvidenceArchNode {
				return true
			}
		}
	}
	return false
}

// failsOnHealthyNode reports a pod whose workload has another pod, on
// another node that is not failing, that fails in one of the same
// modes: the node is not what they share.
func (v *view) failsOnHealthyNode(node, pod inventory.EntityID) bool {
	owner := rootcause.TopOwner(v.s.Model, pod)
	if owner == pod {
		return false
	}
	modes := v.unitModes(pod)
	for _, other := range rootcause.OwnedPods(v.s.Model, owner) {
		at, ok := v.nodeOf(other)
		if other == pod || !ok || at == node || v.failing(at) {
			continue
		}
		for mode := range v.unitModes(other) {
			if modes[mode] {
				return true
			}
		}
	}
	return false
}

// unitModes are the modes of the unhealthy findings of a pod and its
// containers.
func (v *view) unitModes(pod inventory.EntityID) map[detection.Mode]bool {
	out := map[detection.Mode]bool{}
	ids := append([]inventory.EntityID{pod}, v.s.Model.Related(pod,
		inventory.PartOf, inventory.Incoming)...)
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			if unhealthy(f) && f.Mode != "" &&
				!anyModeMatches(summaryModes, f.Mode) {
				out[f.Mode] = true
			}
		}
	}
	return out
}
