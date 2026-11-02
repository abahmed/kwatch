package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// Node bounds: the nodes shown, the conditions one lists and the pods
// read on one node to count the others failing.
const (
	maxNodes          = 3
	maxNodeConditions = 6
	maxNodePods       = 256
)

// capacityLabels are the well-known labels that say whether a node is
// a spot (preemptible) node, with the value meaning spot. Any other
// value of the first two means on-demand.
var capacityLabels = []struct{ key, spot string }{
	{"karpenter.sh/capacity-type", "spot"},
	{"eks.amazonaws.com/capacityType", "SPOT"},
	{"cloud.google.com/gke-spot", "true"},
	{"cloud.google.com/gke-preemptible", "true"},
	{"kubernetes.azure.com/scalesetpriority", "spot"},
}

// Capacity types.
const (
	capacitySpot     = "spot"
	capacityOnDemand = "on-demand"
)

// nodesOf lists the nodes p involves: its root or merged roots when
// they are nodes, and the nodes p's pods run on. Nodes with conditions
// not healthy lead, then those with more of p's pods. cut counts the
// nodes left out.
func nodesOf(model inventory.Reader, p incident.Incident) ([]NodeRow, int) {
	pods := podsOf(model, p)
	mine := map[inventory.EntityID]bool{}
	count := map[inventory.EntityID]int{}
	var nodes []inventory.EntityID
	add := func(id inventory.EntityID) {
		if _, ok := count[id]; !ok {
			nodes = append(nodes, id)
			count[id] = 0
		}
	}
	for _, id := range append([]inventory.EntityID{p.Root}, p.Merged...) {
		if id.Kind == kube.KindNode && id.Name != "" {
			add(id)
		}
	}
	for _, pod := range pods {
		mine[pod] = true
		for _, node := range model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			if node.Kind == kube.KindNode {
				add(node)
				count[node]++
			}
		}
	}
	var out []NodeRow
	for _, id := range nodes {
		if e, ok := model.Entity(id); ok {
			row := nodeRow(model, e, mine)
			row.Root, row.Pods = id == p.Root, count[id]
			out = append(out, row)
		}
	}
	slices.SortFunc(out, compareNodes)
	if len(out) > maxNodes {
		return out[:maxNodes], len(out) - maxNodes
	}
	return out, 0
}

func compareNodes(a, b NodeRow) int {
	root := func(n NodeRow) int {
		if n.Root {
			return 0
		}
		return 1
	}
	return cmp.Or(cmp.Compare(root(a), root(b)),
		cmp.Compare(len(b.Conditions), len(a.Conditions)),
		cmp.Compare(b.Pods, a.Pods), cmp.Compare(a.Name, b.Name))
}

// nodeRow reads one node; mine are p's pods, not counted as others.
func nodeRow(
	model inventory.Reader, e inventory.Entity,
	mine map[inventory.EntityID]bool,
) NodeRow {
	row := NodeRow{Name: e.ID.Name,
		InstanceType: text(e, kube.AttrInstanceType),
		Created:      timeOf(e, kube.AttrCreated),
		Cordoned:     flag(e, kube.AttrUnschedulable),
		Deleting:     flag(e, kube.AttrDeleting),
		Conditions:   nodeConditions(e),
	}
	for _, id := range model.Related(e.ID, inventory.PartOf,
		inventory.Outgoing) {
		switch id.Kind {
		case kube.KindZone:
			row.Zone = id.Name
		case kube.KindNodePool:
			row.Pool = id.Name
		}
	}
	labels, _ := kube.NodeLabels(e, row.Zone)
	row.CapacityType = capacityType(labels)
	if a, ok := e.Attribute(kube.AttrReady); ok {
		if ready, isBool := a.Value.AsBool(); isBool {
			row.Ready = &ready
		}
	}
	row.OtherFailing = otherFailing(model, e.ID, mine)
	return row
}

// capacityType is spot or on-demand by the well-known labels, empty
// when none says.
func capacityType(labels map[string]string) string {
	for i, l := range capacityLabels {
		value, ok := labels[l.key]
		switch {
		case !ok:
		case value == l.spot:
			return capacitySpot
		case i < 2:
			return capacityOnDemand
		}
	}
	return ""
}

// nodeConditions are the node's conditions that are not healthy, by
// type. Ready is healthy when True, the others (pressure, network,
// node-problem-detector conditions) when False; Unknown never is.
func nodeConditions(e inventory.Entity) []NodeCondition {
	var out []NodeCondition
	for name, a := range e.Attributes {
		kind, ok := strings.CutPrefix(name, kube.AttrConditionPrefix)
		if !ok || strings.Contains(kind, ".") {
			continue
		}
		status := a.Value.AsText()
		healthy := "False"
		if kube.HealthyWhenTrue(kind) {
			healthy = "True"
		}
		if status == healthy {
			continue
		}
		key := name
		out = append(out, NodeCondition{Type: kind, Status: status,
			Reason:  text(e, key+kube.AttrConditionReason),
			Message: redact.Evidence(text(e, key+kube.AttrConditionMessage)),
			Since:   timeOf(e, key+kube.AttrConditionSince)})
	}
	slices.SortFunc(out, func(a, b NodeCondition) int {
		return cmp.Compare(a.Type, b.Type)
	})
	if len(out) > maxNodeConditions {
		out = out[:maxNodeConditions]
	}
	return out
}

// otherFailing counts the pods on node, other than mine, that are not
// running well: failed, or not ready while running or pending.
func otherFailing(
	model inventory.Reader, node inventory.EntityID,
	mine map[inventory.EntityID]bool,
) int {
	pods := model.Related(node, inventory.RunsOn, inventory.Incoming)
	slices.SortFunc(pods, compareEntities)
	if len(pods) > maxNodePods {
		pods = pods[:maxNodePods]
	}
	n := 0
	for _, id := range pods {
		if id.Kind != kube.KindPod || mine[id] {
			continue
		}
		if e, ok := model.Entity(id); ok && podFailing(e) {
			n++
		}
	}
	return n
}

// podFailing reports a pod that failed, or that runs or waits without
// being ready. A finished pod is not failing.
func podFailing(e inventory.Entity) bool {
	switch text(e, kube.AttrPhase) {
	case "Failed":
		return true
	case "Succeeded":
		return false
	}
	a, ok := e.Attribute(kube.AttrReady)
	if !ok {
		return false
	}
	ready, _ := a.Value.AsBool()
	return !ready
}

func flag(e inventory.Entity, name string) bool {
	if a, ok := e.Attribute(name); ok {
		v, _ := a.Value.AsBool()
		return v
	}
	return false
}
