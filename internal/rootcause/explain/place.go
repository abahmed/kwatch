package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxPlacePods bounds the pods read to find the container a cause is
// about.
const maxPlacePods = 20

// placeOf says exactly where the cause's setting is: the object, the
// container, the field path and its value now. It is nil when the cause
// is not a setting (a node down, an outside dependency) or when the
// inventory lacks the attributes to name the field. It reads attributes
// and fields only, never text.
func (s Snapshot) placeOf(c Cause, change *inventory.Change) *detection.Place {
	for _, rule := range placeRules {
		if rule.row == c.Row {
			return rule.place(s, c)
		}
	}
	if change != nil {
		return placeOfChange(c.Root, *change)
	}
	return nil
}

// placeRule builds the place of one kind of row.
type placeRule struct {
	row   string
	place func(Snapshot, Cause) *detection.Place
}

// placeRules are the rows whose cause is a setting that is not a
// change: the setting is read from the objects' attributes.
var placeRules = []placeRule{
	{"memory-limit-too-low", Snapshot.memoryLimitPlace},
	{"ephemeral-limit-too-low", Snapshot.ephemeralLimitPlace},
	{"probe-port-mismatch", Snapshot.probePortPlace},
	{"startup-budget-too-short", Snapshot.startupBudgetPlace},
	{"liveness-shorter-than-start", Snapshot.livenessPlace},
	{"liveness-kills-before-ready", Snapshot.livenessPlace},
	{"autoscaling-limit-unavailable", Snapshot.hpaMaxPlace},
	{"budget-blocks-drain", Snapshot.budgetPlace},
	{"routed-missing", Snapshot.routeBackendPlace},
	{"storage-class-missing", Snapshot.claimClassPlace},
	{"claim-not-usable", Snapshot.claimSizePlace},
	{"config-missing-or-changed", Snapshot.missingRefPlace},
	{"used-missing", Snapshot.missingRefPlace},
	{"scheduler-capacity", Snapshot.schedulingPlace},
}

// placeEntities are the entities a cause is about: what it explains,
// or the root when it explains nothing yet.
func placeEntities(c Cause) []inventory.EntityID {
	if len(c.Covers) > 0 {
		return c.Covers
	}
	return []inventory.EntityID{c.Root}
}

// podsOf lists the pods behind an entity: the pod itself, the pod of a
// container, or the pods a workload owns.
func (s Snapshot) podsOf(id inventory.EntityID) []inventory.EntityID {
	switch id.Kind {
	case kube.KindContainer:
		if pod, ok := rootcause.PodOf(s.Model, id); ok {
			return []inventory.EntityID{pod}
		}
		return nil
	case kube.KindPod:
		return []inventory.EntityID{id}
	}
	pods := kube.OwnedPods(s.Model, id)
	return pods[:min(len(pods), maxPlacePods)]
}

// containersOf lists the container entities of what a cause explains.
func (s Snapshot) containersOf(c Cause) []inventory.Entity {
	var out []inventory.Entity
	seen := map[inventory.EntityID]bool{}
	take := func(id inventory.EntityID) {
		if e, ok := s.Model.Entity(id); ok && !seen[id] {
			seen[id] = true
			out = append(out, e)
		}
	}
	for _, id := range placeEntities(c) {
		if id.Kind == kube.KindContainer {
			take(id)
			continue
		}
		for _, pod := range s.podsOf(id) {
			for _, cid := range s.Model.Related(pod, inventory.PartOf,
				inventory.Incoming) {
				take(cid)
			}
		}
	}
	return out
}

// workloadOf is the object a container's setting is written in: the
// top owner of its pod, or the pod itself when nothing owns it.
func (s Snapshot) workloadOf(container inventory.Entity) inventory.EntityID {
	pod, ok := rootcause.PodOf(s.Model, container.ID)
	if !ok {
		return container.ID
	}
	return rootcause.TopOwner(s.Model, pod)
}

// containerName is a container entity's own name: the part of its
// "<pod>/<container>" name after the slash.
func containerName(id inventory.EntityID) string {
	for i := len(id.Name) - 1; i >= 0; i-- {
		if id.Name[i] == '/' {
			return id.Name[i+1:]
		}
	}
	return id.Name
}

// containerPlace builds a place inside one container's settings.
func (s Snapshot) containerPlace(
	container inventory.Entity, path, value string,
) *detection.Place {
	return &detection.Place{Object: s.workloadOf(container),
		Container: containerName(container.ID), Path: path, Value: value}
}

// topOwner is the object that owns id, or id itself.
func (s Snapshot) topOwner(id inventory.EntityID) inventory.EntityID {
	return rootcause.TopOwner(s.Model, id)
}
