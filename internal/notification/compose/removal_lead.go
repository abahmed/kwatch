package compose

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// removalLead leads with what a node's removal broke and says the
// removal is why. A removed node is not "down", and "the 10:10 UTC
// change" does not say what changed: "fleet and storage in shop
// (prod-eu-1) have no ready replicas (0 of 1) because node n3 was
// removed at 10:10 UTC". Without a failing workload the node is the
// subject: "Node n3 (prod-eu-1) was removed at 10:10 UTC".
func removalLead(f caseFacts) (string, bool) {
	change, ok := removedNode(f)
	if !ok {
		return "", false
	}
	workloads := removalSubjects(f)
	if len(workloads) == 0 {
		return f.leadName(change.Entity) + " " + removedAt(change), true
	}
	return causeLink(f.p.Cause, removalSymptom(f, change, workloads),
		removalWhy(f, change)), true
}

// removedAt is "was removed at 10:10 UTC", and by whom when known.
func removedAt(change inventory.Change) string {
	return "was removed at " + clock(change.At) +
		authoredBy(person(change.Actor))
}

// removalWhy is why the workloads fail: "node n3 was removed at 10:10
// UTC", and that no other node has room when a pod could not be placed.
func removalWhy(f caseFacts, change inventory.Change) string {
	why := f.objectName(change.Entity) + " " + removedAt(change)
	if noRoomSeen(f) {
		why += " and no other node has room"
	}
	return why
}

// removalSpread says what newly fails after a node's removal, led by the
// workloads as the first message is: "fleet and storage in shop
// (prod-eu-1) now have no ready replicas (0 of 1) because node n3 was
// removed at 10:10 UTC". Only the first group says why; later ones
// follow it: "Services fleet and storage in shop now have no ready
// backends". ok is false when the node was not removed (a node that is
// NotReady keeps the node as the subject).
func removalSpread(f caseFacts, home inventory.EntityID, first bool,
	g spreadGroup,
) (string, bool) {
	change, ok := removedNode(f)
	if !ok || change.Entity != home {
		return "", false
	}
	text := nameList(home, g.ids)
	if first {
		text += f.clusterTag()
	}
	text += " " + nowStates(g.states, len(g.ids) > 1)
	if first {
		text = causeLink(f.p.Cause, text, removalWhy(f, change))
	}
	return endSentence(capitalName(g.ids[0], text)), true
}

// nowStates joins the states with "now" in the first one, said of one
// subject or of several: "now have no ready replicas (0 of 1) and have
// been Pending since they were scheduled".
func nowStates(states []string, many bool) string {
	out := make([]string, 0, len(states))
	for i, state := range states {
		if i == 0 {
			state = nowState(state)
		}
		if many {
			rest, now := strings.CutPrefix(state, "now ")
			state = pluralPredicate(rest)
			if now {
				state = "now " + state
			}
		}
		out = append(out, state)
	}
	return strings.Join(out, " and ")
}

// removalSymptom is the workloads and what is wrong with them. The one
// workload the cause broke is said as any caused lead says it.
func removalSymptom(f caseFacts, change inventory.Change,
	workloads []inventory.EntityID,
) string {
	if workload := affectedWorkload(f); workload == workloads[0] {
		return symptomState(f, workload)
	}
	return nameList(change.Entity, workloads) + f.clusterTag() + " " +
		removalState(f, workloads)
}

// removedNode is the blamed change when it removed a node.
func removedNode(f caseFacts) (inventory.Change, bool) {
	change := blamedChange(f)
	if change == nil || !change.Deleted ||
		change.Entity.Kind != kube.KindNode {
		return inventory.Change{}, false
	}
	return *change, true
}

// removalSubjects are the workloads the removal lead is about: the
// workload the cause broke, else every failing workload.
func removalSubjects(f caseFacts) []inventory.EntityID {
	if workload := affectedWorkload(f); incident.IsWorkload(workload.Kind) {
		return []inventory.EntityID{workload}
	}
	return removalWorkloads(f)
}

// removalSubject is the first workload removalLead names, the word its
// sentence starts with; ok is false when removalLead does not apply or
// names the node.
func removalSubject(f caseFacts) (inventory.EntityID, bool) {
	if _, ok := removedNode(f); !ok {
		return inventory.EntityID{}, false
	}
	if workloads := removalSubjects(f); len(workloads) > 0 {
		return workloads[0], true
	}
	return inventory.EntityID{}, false
}

// removalWorkloads are the workloads that fail, or run the pods that
// fail, in the incident, once each.
func removalWorkloads(f caseFacts) []inventory.EntityID {
	var ids []inventory.EntityID
	for _, m := range failing(f.members) {
		ids = append(ids, m.Entity)
	}
	var out []inventory.EntityID
	for _, id := range workloadsOf(f, ids) {
		if incident.IsWorkload(id.Kind) {
			out = append(out, id)
		}
	}
	return out
}

// removalState is what the workloads share: their own finding when
// every one of them has the same one, else the incident's state words.
func removalState(f caseFacts, workloads []inventory.EntityID) string {
	states := map[string]bool{}
	covered := map[inventory.EntityID]bool{}
	for _, m := range failing(f.members) {
		if slices.Contains(workloads, m.Entity) {
			states[beforeColon(predicate(m.Entity, m.Summary))] = true
			covered[m.Entity] = true
		}
	}
	state := stateWords(f.p)
	if len(states) == 1 && len(covered) == len(workloads) {
		for only := range states {
			state = only
		}
	}
	if len(workloads) > 1 {
		return pluralPredicate(state)
	}
	return state
}

// noRoomSeen reports a pod the scheduler could place on no node: only
// then does the lead say that no other node has room.
func noRoomSeen(f caseFacts) bool {
	for _, m := range failing(f.members) {
		if m.Mode.Within(detection.ModeUnschedulable) {
			return true
		}
	}
	return false
}
