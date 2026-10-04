package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeSharedFactor is the pseudo mode of a healthy, unchanged entity
// that several workloads failing at the same time have in common: the
// node they all run on. Nothing is known to be wrong with it, so it is
// a suspect, not a cause; the shared-node row gives it a low prior and
// demands several workloads whose failures began together.
const ModeSharedFactor detection.Mode = "SharedFactor"

// sharedFactorModes is the cause side of the shared-node row.
var sharedFactorModes = []detection.Mode{ModeSharedFactor}

// sharedFactorKinds are the kinds workloads share and that can break
// several of them at once while showing nothing themselves.
var sharedFactorKinds = map[inventory.Kind]bool{kube.KindNode: true}

// sharedFactorState is the state of an upstream entity that showed no
// finding and no change: the SharedFactor pseudo mode when its kind is
// one workloads share, nothing otherwise.
func (v *view) sharedFactorState(id inventory.EntityID) state {
	if !sharedFactorKinds[id.Kind] {
		return state{id: id}
	}
	return state{id: id, modes: []modeHealth{{
		mode: ModeSharedFactor, health: detection.Failing, pseudo: true}}}
}

// applySharedFactorWindow keeps shared-factor coverage only when the
// failures it covers began within SharedFactorWindow of each other.
// Three workloads that failed on one node over a week share the node
// by chance; three that failed there within minutes share a cause.
// A failure also stays out unless a replica of its workload runs
// healthy elsewhere: without one, the workload fails the same way on
// any node and is the likelier culprit.
func (v *view) applySharedFactorWindow(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		effects := sharedFactorEffects(c)
		if len(effects) == 0 {
			continue
		}
		reason := v.sharedFactorVeto(c.id, effects)
		if reason != "" {
			v.dropSharedCoverage(cs, c, effects, reason)
			continue
		}
		var lone []inventory.EntityID
		for _, effect := range effects {
			if !v.healthyReplicaElsewhere(effect, c.id) {
				lone = append(lone, effect)
			}
		}
		v.dropSharedCoverage(cs, c, lone, "no replica of the workload "+
			"runs healthy elsewhere, so the workload may be at fault")
	}
}

// dropSharedCoverage removes the shared-factor coverage of effects
// from c, and c itself once nothing else is left to cover.
func (v *view) dropSharedCoverage(
	cs *candidateSet, c *candidate, effects []inventory.EntityID,
	reason string,
) {
	for _, effect := range effects {
		delete(c.covers, effect)
		cs.rejectInsufficient(c.id, effect, reason)
	}
	if len(effects) > 0 && len(c.direct()) == 0 {
		delete(cs.byID, c.id)
	}
}

// healthyReplicaElsewhere reports whether another pod of the effect's
// workload runs healthy without depending on id. Why: a node is only a
// plausible fault for a failure the workload does not repeat on its
// other nodes; a pod with no such replica fails the same way anywhere.
func (v *view) healthyReplicaElsewhere(
	effect, id inventory.EntityID,
) bool {
	for _, sibling := range v.siblings(effect) {
		if !v.unitFailing(sibling) && !v.reaches(sibling, id) {
			return true
		}
	}
	return false
}

// sharedFactorVeto says why shared-factor coverage must be dropped, or
// returns "" when the suspicion stands.
func (v *view) sharedFactorVeto(
	id inventory.EntityID, effects []inventory.EntityID,
) string {
	if !v.beganTogether(effects) {
		return "the failures it would explain did not begin together"
	}
	if !v.mostDependentsFail(id) {
		return "most of what depends on it is healthy"
	}
	return ""
}

// mostDependentsFail reports whether at least SharedFactorMinShare of
// id's dependents fail. Why: a node where most pods are fine is not
// what breaks the failing ones.
func (v *view) mostDependentsFail(id inventory.EntityID) bool {
	dependents := v.dependents(id)
	if len(dependents) == 0 {
		return true
	}
	failing := 0
	for _, pod := range dependents {
		if v.unitFailing(pod) {
			failing++
		}
	}
	return float64(failing)/float64(len(dependents)) >= SharedFactorMinShare
}

// sharedFactorEffects lists the effects a candidate covers through a
// SharedFactor row.
func sharedFactorEffects(c *candidate) []inventory.EntityID {
	var out []inventory.EntityID
	for _, effect := range c.direct() {
		if c.covers[effect].match.causeMode == ModeSharedFactor {
			out = append(out, effect)
		}
	}
	return out
}

// beganTogether reports whether the failures of effects all began
// within SharedFactorWindow of the earliest. Effects without a known
// start are left out of the comparison.
func (v *view) beganTogether(effects []inventory.EntityID) bool {
	var first, last timeValue
	for _, effect := range effects {
		start := v.earliestSince([]inventory.EntityID{effect})
		if !start.ok {
			continue
		}
		first = first.earlier(start.t)
		if !last.ok || start.t.After(last.t) {
			last = timeValue{t: start.t, ok: true}
		}
	}
	return !first.ok || !last.t.After(first.t.Add(SharedFactorWindow))
}
