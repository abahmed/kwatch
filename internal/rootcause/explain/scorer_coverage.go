package explain

import (
	"fmt"
	"slices"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreCoverage compares the candidate's failing dependents with all
// of them. A node whose every pod fails is a better cause than one
// where one pod in twenty fails.
func scoreCoverage(v *view, c *candidate) outcome {
	// An entity blamed only for itself is judged by its own finding;
	// what its dependents do says nothing about that.
	if len(c.others()) == 0 {
		return outcome{}
	}
	// Every dependent is counted: a sample would put "31 of 32" in a
	// message about a node that runs a hundred pods.
	dependents := v.dependents(c.id)
	if len(dependents) < CoverageMinDependents {
		return outcome{}
	}
	failing := 0
	for _, id := range dependents {
		if v.unitFailing(id) {
			failing++
		}
	}
	share := float64(failing) / float64(len(dependents))
	weight := CoverageWeight * (2*share - 1)
	if weight > 0 && c.selfLinked() {
		// The pods of a workload blamed for its own setting are the
		// failures themselves: they cannot also be support for it.
		return outcome{}
	}
	return outcome{weight: weight,
		code: rootcause.ProofDependentsFail, count: failing,
		total: len(dependents), text: fmt.Sprintf("%d of %d dependents fail", failing,
			len(dependents))}
}

// scoreSelfQuoted rewards a workload blamed for itself whose every
// dependent fails and quotes its own error: the program says what is
// wrong with it in every replica, which is more than "nothing upstream
// is failing".
func scoreSelfQuoted(v *view, c *candidate) outcome {
	if len(c.others()) == 0 || !c.isSelf() {
		return outcome{}
	}
	dependents := v.dependents(c.id)
	if len(dependents) < CoverageMinDependents {
		return outcome{}
	}
	for _, id := range dependents {
		if !v.unitFailing(id) {
			return outcome{}
		}
	}
	// A failure something upstream explains is that cause's evidence,
	// whatever the program says about it.
	for _, effect := range c.others() {
		if v.explainedOutside[effect] {
			return outcome{}
		}
	}
	if !v.ownErrorsQuoted(dependents) {
		return outcome{}
	}
	n := len(dependents)
	return outcome{weight: SelfQuotedWeight,
		code: rootcause.ProofOwnErrors, count: n, total: n,
		text: fmt.Sprintf("%d of %d dependents fail with their own "+
			"error", n, n)}
}

// ownErrorsQuoted reports that each of ids, or a container of it,
// quotes the program's own error: the words it exited with or its last
// log line.
func (v *view) ownErrorsQuoted(ids []inventory.EntityID) bool {
	for _, id := range ids {
		units := append([]inventory.EntityID{id}, v.s.Model.Related(
			id, inventory.PartOf, inventory.Incoming)...)
		if !slices.ContainsFunc(units, v.quotesOwnError) {
			return false
		}
	}
	return len(ids) > 0
}

// quotesOwnError reports an unhealthy finding of id that quotes the
// program's own error: its error line, or the termination message of a
// container that exited with an error. A previous log line no finding
// quotes counts too. Messages the kubelet writes do not, nor does an
// error of a call to another address ("dial tcp ...: connection
// refused"): that is about the dependency, not the program.
func (v *view) quotesOwnError(id inventory.EntityID) bool {
	for _, f := range v.s.Findings[id] {
		if !unhealthy(f) {
			continue
		}
		for _, e := range f.Evidence {
			if e.Value != "" && (e.Label == detection.EvidenceError ||
				e.Label == "message" && f.Reason == reasons.Error &&
					id.Kind == kube.KindContainer) &&
				!hasMarker(e.Value, dependencyErrorMarkers) {
				return true
			}
		}
	}
	line := v.previousLogLine(id)
	return line != "" && !hasMarker(line, dependencyErrorMarkers)
}

// dependents lists what depends on id directly: the pods of a node or
// an owner, the nodes of a zone, the users of a secret. Virtual kinds
// other than zones and pools have no stored dependents.
func (v *view) dependents(id inventory.EntityID) []inventory.EntityID {
	switch id.Kind {
	case kube.KindNode:
		return v.s.Model.Related(id, inventory.RunsOn, inventory.Incoming)
	case kube.KindZone, kube.KindNodePool:
		return v.s.Model.Related(id, inventory.PartOf, inventory.Incoming)
	case kube.KindPod:
		return nil
	case kube.KindNetworkPolicy:
		// A policy depends on nothing; its dependents are the pods it
		// selects.
		return v.selectedBy(id)
	}
	if pods := rootcause.OwnedPods(v.s.Model, id); len(pods) > 0 {
		return pods
	}
	var out []inventory.EntityID
	for _, relation := range v.s.Model.Relations(id, inventory.Incoming) {
		if relation.From.Kind == kube.KindPod {
			out = append(out, relation.From)
		}
	}
	return out
}

// selectedBy lists the pods an entity's selector matches.
func (v *view) selectedBy(id inventory.EntityID) []inventory.EntityID {
	if v.s.Links == nil {
		return nil
	}
	var out []inventory.EntityID
	for _, link := range v.s.Links.Links(id) {
		if link.Type == inventory.Selects && link.To.Kind == kube.KindPod {
			out = append(out, link.To)
		}
	}
	return out
}
