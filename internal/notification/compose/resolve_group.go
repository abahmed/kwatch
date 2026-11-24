package compose

import (
	"slices"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// sharedErrorGroup reports an incident rooted at a shared error: its
// root is the error several workloads fail with, not an object.
func sharedErrorGroup(p incident.Incident) bool {
	return p.Root.Kind == explain.KindFailureSignature
}

// resolveSubject is what a resolve is about. A shared-error group ends
// on its workloads: a subject that is not one of them (an object a
// listing named before the group took its failures over) is not what
// recovered, so the group's first workload stands in.
func resolveSubject(f caseFacts) inventory.EntityID {
	subject := leadSubject(f)
	if !sharedErrorGroup(f.p) || slices.Contains(f.p.Impact, subject) {
		return subject
	}
	for _, id := range f.p.Impact {
		if incident.IsWorkload(id.Kind) {
			return id
		}
	}
	return subject
}

// resolveLead is the lead of a resolve: "payments in shop is healthy
// again.", and for a shared-error group with others, "cart in shop and
// five other workloads are healthy again."
func resolveLead(f caseFacts, subject inventory.EntityID) string {
	state := resolution(f, subject) + stillClause(f, subject)
	name := f.leadName(subject)
	if n := otherWorkloads(f, subject); sharedErrorGroup(f.p) && n > 0 {
		name += " and " + plural(n, "other workload")
		state = pluralPredicate(state)
	}
	return capitalName(subject, name+" "+state+".")
}
