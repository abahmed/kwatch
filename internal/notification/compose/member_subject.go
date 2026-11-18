package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// aboutLead reports whether "it" and "its" in a sentence about the
// member id read as the lead's own subject (owns). For any other member
// (a container beside a node or Secret lead) the sentence must name it.
func (f caseFacts) aboutLead(id inventory.EntityID) bool {
	return f.owns(leadSubject(f), id)
}

// owns reports whether a finding of id may be told as subject's own:
// id is subject, or part of it by the cluster's relations
// (incident.Belongs): a pod, container or job it runs, its own
// autoscaler, a Service or disruption budget that selects only its
// pods. Every sentence that puts a member's finding on a subject (the
// lead's state, a probe or CPU lead, "it" in a proof) asks this
// first: a node is never "not ready: its readiness check fails"
// because a container on it is, and a Secret never "exits". A name
// alone never makes an object of another kind the subject's.
func (f caseFacts) owns(subject, id inventory.EntityID) bool {
	if owned, known := f.p.Belongs.Of(subject, id); known {
		// A pod is told as its controller's own only when that
		// controller runs code: an operator's resource names it.
		return owned && (id == subject || !createdKinds[id.Kind] ||
			incident.IsApplicationKind(subject.Kind))
	}
	return ownsByName(subject, id)
}

// ownsByName is owns for an object the incident holds no relation for
// (an older record): only what a controller creates and names after
// itself, a pod, container or job, counts as the subject's.
func ownsByName(subject, id inventory.EntityID) bool {
	if id == subject {
		return true
	}
	if id.Kind == kube.KindContainer {
		pod, _ := splitContainer(id.Name)
		id = inventory.CoreID(kube.KindPod, id.Namespace, pod)
		if id == subject {
			return true
		}
	}
	if owner, ok := ownerIn([]inventory.EntityID{subject}, id); ok {
		return owner == subject
	}
	return createdKinds[id.Kind] && subject.Kind != kube.KindContainer &&
		incident.IsApplicationKind(subject.Kind) &&
		id.Namespace == subject.Namespace && subject.Name != "" &&
		strings.HasPrefix(id.Name, subject.Name+"-")
}

// createdKinds are the kinds a controller creates and names after
// itself.
var createdKinds = map[inventory.Kind]bool{
	kube.KindPod: true, kube.KindContainer: true, kube.KindJob: true,
	kube.KindReplicaSet: true,
}

// othersState is what follows subject when the lead's finding is
// another object's (owns): with one failing object, its finding said
// of it ("is failing; secret tls has expired"), with more, how many
// workloads they run under ("is failing, affecting two workloads"),
// the members listed after the lead.
func (f caseFacts) othersState(subject inventory.EntityID) string {
	text := stateWords(f.p)
	if n := f.affected(subject); n > 1 {
		return text + ", affecting " + plural(n, "workload")
	}
	return text + "; " + f.memberName(f.lead.Entity) + " " +
		beforeColon(leadState(f))
}

// affected counts the workloads other than subject that the failing
// members run under (a pod or job of none counts on its own).
func (f caseFacts) affected(subject inventory.EntityID) int {
	seen := map[inventory.EntityID]bool{}
	for _, m := range failing(f.members) {
		id := m.Entity
		if owner, ok := ownerIn(f.p.Impact, id); ok {
			id = owner
		}
		if !f.owns(subject, id) && incident.IsApplicationKind(id.Kind) {
			seen[id] = true
		}
	}
	return len(seen)
}

// memberName names the member a sentence is about, relative to the
// lead's root: "container app in pod api-7d9f in shop".
func (f caseFacts) memberName(id inventory.EntityID) string {
	return nameFrom(f.p.Root, id)
}
