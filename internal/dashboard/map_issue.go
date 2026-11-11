package dashboard

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// maxIssueMapNodes bounds an issue's picture: the objects it touches
// and the ones next to them, never a whole namespace.
const maxIssueMapNodes = 24

// IssueMap is the small picture on an issue's Impact tab: the objects
// the issue touches (its root and members, folded into their workloads)
// and the objects one connection away, from relations every cluster has
// (Ingress or route to Service, Service to workload) and, where kwatch
// knows them, the Services a workload calls. Self is the issue's own
// object, drawn highlighted.
type IssueMap struct {
	Self  string    `json:"self"`
	Nodes []MapNode `json:"nodes"`
	Edges []MapEdge `json:"edges"`
}

// MapNode is one object: a route (Ingress, HTTPRoute), a Service or a
// workload with its pods folded in. Level is the tier of the worst open
// issue touching it ("page", "notify", "digest") or "ok"; Issue is that
// issue's ID. External marks an object of another namespace.
type MapNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Level     string `json:"level"`
	Ready     string `json:"ready,omitempty"`
	Issue     string `json:"issue,omitempty"`
	External  bool   `json:"external,omitempty"`
}

// MapEdge is one connection: "routes" (route to Service), "selects"
// (Service to workload) or "calls" (workload to a Service it calls).
type MapEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// issueMap draws p's picture, or nil when kwatch knows no connection
// of the objects p touches, or they are out of the dashboard's scope.
func issueMap(
	model inventory.Reader, scope Scope, open []incident.Incident,
	p incident.Incident,
) *IssueMap {
	b := newMapBuilder(model, scope, false)
	b.markLevels(open)
	self := b.fold(p.Root)
	g := b.namespace(self.Namespace)
	focus := map[string]bool{nodeID(self): true}
	for key := range p.Members {
		focus[nodeID(b.fold(key.Entity))] = true
	}
	keep := map[string]bool{}
	var edges []MapEdge
	for e := range g.edges {
		if focus[e.From] || focus[e.To] {
			edges = append(edges, e)
			keep[e.From], keep[e.To] = true, true
		}
	}
	if len(edges) == 0 || len(keep) > maxIssueMapNodes {
		return nil
	}
	out := &IssueMap{Self: nodeID(self), Edges: edges}
	for key := range keep {
		out.Nodes = append(out.Nodes, g.nodes[key])
	}
	slices.SortFunc(out.Nodes, func(a, b MapNode) int {
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(out.Edges, func(a, b MapEdge) int {
		if c := strings.Compare(a.From, b.From); c != 0 {
			return c
		}
		return strings.Compare(a.To, b.To)
	})
	return out
}
