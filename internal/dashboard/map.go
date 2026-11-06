package dashboard

import (
	"net/http"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// maxMapNodes bounds what one map draws: past it the answer says so and
// the page asks for a namespace, so a large cluster is never a tangle.
const maxMapNodes = 150

// Map views: one namespace, or what fails across the namespaces kwatch
// watches with the objects next to it.
const (
	mapViewNamespace = "namespace"
	mapViewFailing   = "failing"
)

// mapLevelOK is the level of an object no open issue touches.
const mapLevelOK = "ok"

// MapResponse is the /api/map response: the objects of one namespace
// (or what fails) and how they connect, from relations every cluster
// has (Ingress or route to Service, Service to workload) and, where
// kwatch knows them, the Services a workload calls.
type MapResponse struct {
	View      string `json:"view"`
	Namespace string `json:"namespace,omitempty"`
	// Namespaces are the watched namespaces with objects to draw, for
	// the picker.
	Namespaces []string  `json:"namespaces"`
	Nodes      []MapNode `json:"nodes"`
	Edges      []MapEdge `json:"edges"`
	// TooMany says the map has more than maxMapNodes objects; Nodes and
	// Edges are then empty and Count says how many there were.
	TooMany bool `json:"tooMany,omitempty"`
	Count   int  `json:"count"`
}

// MapNode is one object: a route (Ingress, HTTPRoute), a Service, a
// workload with its pods folded in, or a node. Level is the tier of the
// worst open issue touching it ("page", "notify", "digest") or "ok";
// Issue is that issue's ID. External marks an object of another
// namespace that one of this namespace's workloads calls.
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
// (Service to workload), "calls" (workload to a Service it calls) or
// "runs-on" (workload to node, with ?nodes=1).
type MapEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// mapHandler serves GET /api/map?view=namespace|failing&namespace=&
// nodes=1. Without a namespace the namespace view picks the namespace
// with the most failing objects, else the first.
func (s *Server) mapHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	q := r.URL.Query()
	view := q.Get("view")
	if view == "" {
		view = mapViewNamespace
	}
	if (view != mapViewNamespace && view != mapViewFailing) ||
		len(q["view"]) > 1 {
		writeBadQuery(w, badQuery("view", view,
			"Unknown map view: %q. Use namespace or failing.", view))
		return
	}
	nodes := q.Get("nodes")
	if (nodes != "" && nodes != "0" && nodes != "1") || len(q["nodes"]) > 1 {
		writeBadQuery(w, badQuery("nodes", nodes,
			"Unknown nodes value: %q. Use 1 or 0.", nodes))
		return
	}
	model, reader := s.sources.model.Load(), s.sources.incidents.Load()
	if model == nil || reader == nil {
		unavailable(w)
		return
	}
	b := newMapBuilder(*model, s.opts.Scope, nodes == "1")
	b.markLevels((*reader).Incidents())
	out := b.build(view, q.Get("namespace"))
	writeTagged(w, r, out)
}

// mapScope reports a namespace the map may show: watched and not
// excluded. Cluster-scoped objects (namespace "") always pass.
func mapScope(scope Scope, namespace string) bool {
	if namespace == "" {
		return true
	}
	if slices.Contains(scope.Excluded, namespace) {
		return false
	}
	return len(scope.Watched) == 0 ||
		slices.Contains(scope.Watched, namespace)
}

// tierRank orders the levels a node can take; ok is the lowest.
func tierRank(level string) int {
	switch level {
	case incident.Page.String():
		return 3
	case incident.Notify.String():
		return 2
	case incident.Digest.String():
		return 1
	}
	return 0
}

// nodeID is a map node's stable ID.
func nodeID(id inventory.EntityID) string { return id.String() }

// sortMap orders nodes and edges so two reads of the same model give
// the same body (and ETag).
func sortMap(out *MapResponse) {
	slices.SortFunc(out.Nodes, func(a, b MapNode) int {
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(out.Edges, func(a, b MapEdge) int {
		if c := strings.Compare(a.From, b.From); c != 0 {
			return c
		}
		if c := strings.Compare(a.To, b.To); c != 0 {
			return c
		}
		return strings.Compare(a.Type, b.Type)
	})
}
