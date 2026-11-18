package dashboard

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Why a change is listed on the issue page, in plain words. Each
// sentence comes from a link the model holds between the changed object
// and the failing objects; a change with no such link gets no sentence
// rather than a vague one.

// maxWhyPods bounds the pods examined for one issue, so a Deployment
// of thousands of replicas costs no more than a small one.
const maxWhyPods = 16

// changeLinks is what the failing objects of one issue are, and what
// they use, built once per page and asked about each change.
type changeLinks struct {
	model    inventory.Reader
	failing  map[inventory.EntityID]bool
	pods     map[inventory.EntityID]bool
	owners   map[inventory.EntityID]bool
	nodes    map[inventory.EntityID]bool
	mounted  map[inventory.EntityID]bool
	used     map[inventory.EntityID]bool
	causeIDs map[inventory.EntityID]bool
}

// newChangeLinks collects the failing objects of p: its members (or
// its root when it has none), their pods, the workloads that own them
// and the nodes the pods run on. Nil without a model: stored changes
// are listed without a reason then.
func newChangeLinks(
	model inventory.Reader, p incident.Incident,
) *changeLinks {
	if model == nil {
		return nil
	}
	l := &changeLinks{model: model,
		failing: map[inventory.EntityID]bool{},
		pods:    map[inventory.EntityID]bool{},
		owners:  map[inventory.EntityID]bool{},
		nodes:   map[inventory.EntityID]bool{},
		mounted: map[inventory.EntityID]bool{},
		used:    map[inventory.EntityID]bool{}, causeIDs: causeIDs(p)}
	for _, id := range failingIDs(p) {
		l.failing[id] = true
		l.addPod(id)
	}
	l.collectUses()
	return l
}

// failingIDs are the objects of p's failing members, or its root when
// it has none.
func failingIDs(p incident.Incident) []inventory.EntityID {
	var out []inventory.EntityID
	for _, f := range p.Members {
		if !f.Advisory {
			out = append(out, f.Entity)
		}
	}
	if len(out) == 0 && p.Root.Kind != "" {
		out = append(out, p.Root)
	}
	return out
}

// causeIDs are the objects the likely cause names: its root and the
// object of the change it blames.
func causeIDs(p incident.Incident) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	if p.Cause == nil {
		return out
	}
	out[p.Cause.Root] = true
	if p.Cause.Change != nil {
		out[p.Cause.Change.Entity] = true
	}
	return out
}

// addPod records the pod of a failing object, the workloads above it
// and the node it runs on. A failing workload counts its own pods.
func (l *changeLinks) addPod(id inventory.EntityID) {
	base := id
	if pod, ok := rootcause.PodOf(l.model, id); ok {
		base = pod
		l.notePod(pod)
	}
	for _, owner := range rootcause.OwnerChain(l.model, base) {
		l.owners[owner] = true
	}
	if base == id && id.Kind != kube.KindPod {
		for _, pod := range rootcause.OwnedPods(l.model, id) {
			l.notePod(pod)
		}
	}
}

func (l *changeLinks) notePod(pod inventory.EntityID) {
	if len(l.pods) >= maxWhyPods || l.pods[pod] {
		return
	}
	l.pods[pod] = true
	for _, node := range l.model.Related(pod, inventory.RunsOn,
		inventory.Outgoing) {
		l.nodes[node] = true
	}
}

// collectUses records what the failing objects, their pods and their
// workloads mount and reference.
func (l *changeLinks) collectUses() {
	for _, set := range []map[inventory.EntityID]bool{
		l.failing, l.pods, l.owners,
	} {
		for id := range set {
			for _, m := range l.model.Related(id, inventory.Mounts,
				inventory.Outgoing) {
				l.mounted[m] = true
			}
			for _, r := range l.model.Related(id, inventory.References,
				inventory.Outgoing) {
				l.used[r] = true
			}
		}
	}
}

// why says why change is listed, or "" when no link is known.
func (l *changeLinks) why(change inventory.Change) string {
	if l == nil {
		return ""
	}
	if why, ok := l.endedWhy(change); ok {
		return why
	}
	id := change.Entity
	switch {
	case l.pods[id]:
		return "This is one of the failing pods."
	case l.owners[id]:
		return ownerWhy(change)
	case l.failing[id]:
		return failingWhy(change)
	case l.causeIDs[id]:
		return "This is the object the likely cause names."
	case l.mounted[id]:
		return "The failing pods mount this " + kindWord(id.Kind) + "."
	case l.used[id]:
		return "The failing pods use this " + kindWord(id.Kind) + "."
	case l.nodes[id]:
		return "This is the node the failing pods run on."
	}
	return l.frontWhy(id)
}

// failingWhy is the sentence for a failing object's own change: a
// failing workload's template edit says so, like ownerWhy.
func failingWhy(change inventory.Change) string {
	if templateEdit(change) {
		return "This is the workload's own template."
	}
	return "This is the failing object itself."
}

// templateEdit reports a rollout or image edit: one that changes the
// pod template, so the pods themselves.
func templateEdit(change inventory.Change) bool {
	switch change.Classify() {
	case inventory.ClassRollout, inventory.ClassImage:
		return true
	}
	return false
}

// ownerWhy is the sentence for a workload above the failing pods: a
// rollout or image edit changes the pod template, so the pods
// themselves.
func ownerWhy(change inventory.Change) string {
	if templateEdit(change) {
		return "This is the workload's own template."
	}
	return "This is the workload the failing pods belong to."
}
