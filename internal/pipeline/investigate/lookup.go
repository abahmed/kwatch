package investigate

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Report reads bounds per incident.
const (
	maxReportPods       = 3
	maxReportContainers = 4
	maxNotes            = 30
	maxNodePods         = 40
)

// incidentPods are the pods the report reads: the member pods, else the
// pods of the root workload, at most maxReportPods, each once.
func incidentPods(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	pods := memberPods(model, p)
	if len(pods) == 0 {
		pods = workloadPods(model, p.Root)
	}
	return firstUnique(pods, maxReportPods)
}

// workloadPods are the pods a workload owns, directly or through its
// ReplicaSets.
func workloadPods(
	model inventory.Reader, id inventory.EntityID,
) []inventory.EntityID {
	if id.Kind == kube.KindPod {
		return []inventory.EntityID{id}
	}
	var pods []inventory.EntityID
	for _, owned := range model.Related(id, inventory.OwnedBy,
		inventory.Incoming) {
		switch owned.Kind {
		case kube.KindPod:
			pods = append(pods, owned)
		case kube.KindReplicaSet:
			pods = append(pods, model.Related(owned, inventory.OwnedBy,
				inventory.Incoming)...)
		}
	}
	sortIDs(pods)
	return pods
}

// containersOf are the containers of pod, by name.
func containersOf(
	model inventory.Reader, pod inventory.EntityID,
) []inventory.EntityID {
	out := model.Related(pod, inventory.PartOf, inventory.Incoming)
	sortIDs(out)
	return firstUnique(out, maxReportContainers)
}

// workloadOf is the workload the incident is about: the root when it
// is one, else the top owner of its first pod.
func workloadOf(
	model inventory.Reader, p incident.Incident,
) (inventory.EntityID, bool) {
	if incident.IsWorkload(p.Root.Kind) && p.Root.Kind != kube.KindPod {
		return p.Root, true
	}
	pods := incidentPods(model, p)
	if len(pods) == 0 {
		return inventory.EntityID{}, false
	}
	owner, ok := ownerOf(model, pods[0])
	if ok && owner.Kind == kube.KindReplicaSet {
		if top, up := ownerOf(model, owner); up {
			return top, true
		}
	}
	return owner, ok
}

func ownerOf(
	model inventory.Reader, id inventory.EntityID,
) (inventory.EntityID, bool) {
	owners := model.Related(id, inventory.OwnedBy, inventory.Outgoing)
	if len(owners) == 0 {
		return inventory.EntityID{}, false
	}
	sortIDs(owners)
	return owners[0], true
}

// relatedOf returns the entities of kind related to id, sorted.
func relatedOf(
	model inventory.Reader, id inventory.EntityID,
	rel inventory.RelationType, dir inventory.Direction, kind inventory.Kind,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, r := range model.Related(id, rel, dir) {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	sortIDs(out)
	return out
}

// failingConditions lists the entity's conditions that are not in their
// healthy state: "Ready=False (KubeletNotReady): message".
func failingConditions(e inventory.Entity) []string {
	var names []string
	for name := range e.Attributes {
		rest, ok := strings.CutPrefix(name, kube.AttrConditionPrefix)
		if ok && !strings.Contains(rest, ".") {
			names = append(names, rest)
		}
	}
	sort.Strings(names)
	var out []string
	for _, c := range names {
		status := conditionText(e, c, "")
		if healthyCondition(c, status) {
			continue
		}
		text := c + "=" + status
		if reason := conditionText(e, c,
			kube.AttrConditionReason); reason != "" {
			text += " (" + reason + ")"
		}
		if msg := conditionText(e, c,
			kube.AttrConditionMessage); msg != "" {
			text += ": " + msg
		}
		out = append(out, text)
	}
	return out
}

// healthyCondition reports a condition in its good state: pressure
// conditions are good when False, the others when True.
func healthyCondition(name, status string) bool {
	for _, c := range pressureConditions {
		if c == name {
			return status != "True"
		}
	}
	return status == "True"
}

// textOf is an attribute of e as text, empty when unset.
func textOf(e inventory.Entity, name string) string {
	if a, ok := e.Attribute(name); ok {
		return strings.TrimSpace(a.Value.AsText())
	}
	return ""
}

// notesOf returns the notes of ids since the incident's lookback, the
// newest first, at most maxNotes, as dashboard lines.
func notesOf(
	model inventory.Reader, p incident.Incident, ids []inventory.EntityID,
) []string {
	type dated struct {
		note inventory.Note
		of   inventory.EntityID
	}
	var all []dated
	since := p.Opened.Add(-configLookback)
	for _, id := range firstUnique(ids, len(ids)) {
		for _, n := range model.Notes(id, since) {
			all = append(all, dated{note: n, of: id})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].note.At.After(all[j].note.At)
	})
	var out []string
	for _, d := range all[:min(len(all), maxNotes)] {
		out = append(out, noteLine(d.of, d.note))
	}
	return out
}

// noteLine is "13:42:07 pod/api-1 BackOff (x4): message".
func noteLine(of inventory.EntityID, n inventory.Note) string {
	line := n.At.UTC().Format("15:04:05") + " " + string(of.Kind) + "/" +
		of.Name + " " + n.Reason
	if n.Count > 1 {
		line += " (x" + strconv.Itoa(n.Count) + ")"
	}
	if n.Message != "" {
		line += ": " + n.Message
	}
	return line
}

func sortIDs(ids []inventory.EntityID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}

// firstUnique keeps the first n distinct ids.
func firstUnique(ids []inventory.EntityID, n int) []inventory.EntityID {
	seen := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, id := range ids {
		if !seen[id] && len(out) < n {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
