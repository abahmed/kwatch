package status

import (
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// minZoneFailures is how many failures (nodes not ready plus failing
// pods) one zone needs before it counts as concentrating them, unless
// every node of the zone is down.
const minZoneFailures = 3

// ZoneHealth counts, per availability zone, the nodes that are not
// ready and the failing pods that run on the zone's nodes. A zone is
// Concentrated only when the separation is clean: the cluster has two
// or more zones with nodes, every failure is in this one zone, and the
// others have no failing node and no failing pod.
func ZoneHealth(
	model inventory.Reader, findings []detection.Finding,
) Zones {
	if model == nil {
		return Zones{}
	}
	byZone := map[string]*Zone{}
	zoneOfNode := map[inventory.EntityID]string{}
	for _, id := range model.Entities(kube.KindZone) {
		if id.Name == "" {
			continue
		}
		z := &Zone{Name: id.Name}
		for _, node := range model.Related(
			id, inventory.PartOf, inventory.Incoming) {
			z.Nodes++
			zoneOfNode[node] = id.Name
			if nodeNotReady(model, node) {
				z.NotReady++
			}
		}
		if z.Nodes > 0 {
			byZone[id.Name] = z
		}
	}
	countFailingPods(model, findings, zoneOfNode, byZone)
	return assessZones(byZone)
}

// nodeNotReady reports a node whose Ready condition is known to be
// false. A node that has not said is not counted.
func nodeNotReady(model inventory.Reader, id inventory.EntityID) bool {
	e, ok := model.Entity(id)
	if !ok {
		return false
	}
	attribute, ok := e.Attribute(kube.AttrReady)
	if !ok {
		return false
	}
	ready, known := attribute.Value.AsBool()
	return known && !ready
}

// countFailingPods adds each failing pod to the zone of its node. A pod
// no node runs yet belongs to no zone.
func countFailingPods(
	model inventory.Reader, findings []detection.Finding,
	zoneOfNode map[inventory.EntityID]string, byZone map[string]*Zone,
) {
	seen := map[inventory.EntityID]bool{}
	for _, f := range findings {
		if f.Advisory || f.Entity.Kind != kube.KindPod || seen[f.Entity] {
			continue
		}
		seen[f.Entity] = true
		for _, node := range model.Related(
			f.Entity, inventory.RunsOn, inventory.Outgoing) {
			if z := byZone[zoneOfNode[node]]; z != nil {
				z.FailingPods++
			}
		}
	}
}

// assessZones orders the zones, failing ones first, and marks the one
// that concentrates every failure.
func assessZones(byZone map[string]*Zone) Zones {
	if len(byZone) < 2 {
		return Zones{}
	}
	out := Zones{Assessed: true}
	var failing []*Zone
	for _, z := range byZone {
		out.Items = append(out.Items, *z)
		if z.NotReady+z.FailingPods > 0 {
			failing = append(failing, z)
		}
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if (a.NotReady+a.FailingPods > 0) != (b.NotReady+b.FailingPods > 0) {
			return a.NotReady+a.FailingPods > 0
		}
		return a.Name < b.Name
	})
	if len(failing) == 1 && concentrates(*failing[0]) {
		for i := range out.Items {
			out.Items[i].Concentrated = out.Items[i].Name == failing[0].Name
		}
	}
	if len(out.Items) > MaxZones {
		out.Items = out.Items[:MaxZones]
	}
	return out
}

// concentrates reports a zone with enough failures to be the story.
func concentrates(z Zone) bool {
	return z.NotReady == z.Nodes || z.NotReady+z.FailingPods >= minZoneFailures
}
