package dashboard

import (
	"cmp"
	"slices"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// InsightNode is a node with active issues on it: Issues counts the
// issues whose pods run there (or that are about the node itself),
// Urgent the urgent ones among them.
type InsightNode struct {
	Name   string `json:"name"`
	Issues int    `json:"issues"`
	Urgent int    `json:"urgent"`
}

// nodeNames are the nodes p involves: its root and merged roots when
// they are nodes, and the nodes its pods run on. Sorted, no repeats.
func nodeNames(model inventory.Reader, p incident.Incident) []string {
	seen := map[string]bool{}
	for _, id := range append([]inventory.EntityID{p.Root}, p.Merged...) {
		if id.Kind == kube.KindNode && id.Name != "" {
			seen[id.Name] = true
		}
	}
	for _, pod := range podsOf(model, p) {
		for _, node := range model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			if node.Kind == kube.KindNode {
				seen[node.Name] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// busyNodes are the insightTop nodes with the most active issues; none
// without the inventory.
func busyNodes(
	model inventory.Reader, live []incident.Incident,
) []InsightNode {
	out := make([]InsightNode, 0)
	if model == nil {
		return out
	}
	byName := map[string]*InsightNode{}
	for _, p := range live {
		for _, name := range nodeNames(model, p) {
			n := byName[name]
			if n == nil {
				n = &InsightNode{Name: name}
				byName[name] = n
			}
			n.Issues++
			if p.Tier == incident.Page {
				n.Urgent++
			}
		}
	}
	for _, n := range byName {
		out = append(out, *n)
	}
	slices.SortFunc(out, func(a, b InsightNode) int {
		return cmp.Or(b.Issues-a.Issues, b.Urgent-a.Urgent,
			cmp.Compare(a.Name, b.Name))
	})
	return out[:min(len(out), insightTop)]
}

// onNodes is the IDs of the incidents that involve one of nodes, for
// the list's node filter; empty without the inventory, so the filter
// then matches nothing.
func onNodes(
	model inventory.Reader, items []incident.Incident,
	nodes map[string]bool,
) map[string]bool {
	out := map[string]bool{}
	if model == nil {
		return out
	}
	for i, p := range items {
		if i >= maxInsightScan {
			break
		}
		for _, name := range nodeNames(model, p) {
			if nodes[name] {
				out[p.ID] = true
				break
			}
		}
	}
	return out
}
