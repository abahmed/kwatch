package context

import "sync"

type Edge struct {
	From string
	To   string
	Type string
}

// EdgeTarget describes one outgoing relationship when a resource replaces its
// graph contribution as an atomic snapshot.
type EdgeTarget struct {
	Kind      string
	Namespace string
	Name      string
	Type      string
}

type ResourceGraph struct {
	mu           sync.RWMutex
	dependents   map[string]map[string]bool
	dependencies map[string]map[string]bool
	edges        map[string]Edge
	edgeCounts   map[string]int
}

func NewResourceGraph() *ResourceGraph {
	return &ResourceGraph{
		dependents:   make(map[string]map[string]bool),
		dependencies: make(map[string]map[string]bool),
		edges:        make(map[string]Edge),
		edgeCounts:   make(map[string]int),
	}
}

// Size reports how many resources and relationships the graph currently
// holds. An empty or shrinking graph means diagnoses will be empty too, and
// nothing else makes that visible.
func (g *ResourceGraph) Size() (nodes, edges int) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	seen := make(map[string]struct{}, len(g.dependents)+len(g.dependencies))
	for k := range g.dependents {
		seen[k] = struct{}{}
	}
	for k := range g.dependencies {
		seen[k] = struct{}{}
	}
	return len(seen), len(g.edges)
}

func resourceKey(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

func edgeKey(from, to, edgeType string) string {
	return relationshipKey(from, to) + "\x00" + edgeType
}

func relationshipKey(from, to string) string {
	return from + "\x00" + to
}

func (g *ResourceGraph) AddEdge(
	fromKind, fromNS, fromName, toKind, toNS, toName, edgeType string,
) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.addEdgeLocked(Edge{
		From: resourceKey(fromKind, fromNS, fromName),
		To:   resourceKey(toKind, toNS, toName),
		Type: edgeType,
	})
}

// ReplaceMatchingEdges atomically removes edges selected by match and adds the
// supplied replacements. It lets an informer stage a resource's replacement
// relationships before changing the live graph, without deleting edges that
// are owned by other resource builders.
func (g *ResourceGraph) ReplaceMatchingEdges(match func(Edge) bool, additions []Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, edge := range g.edges {
		if match(edge) {
			g.removeEdgeLocked(edge)
		}
	}
	for _, edge := range additions {
		g.addEdgeLocked(edge)
	}
}

// ReplaceOutgoingEdges atomically replaces every relationship contributed by
// a resource. Informer updates use this after their new relationships have
// been staged so graph readers never see a partially rebuilt resource.
func (g *ResourceGraph) ReplaceOutgoingEdges(
	kind, namespace, name string, targets []EdgeTarget,
) {
	from := resourceKey(kind, namespace, name)
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, edge := range g.edges {
		if edge.From == from {
			g.removeEdgeLocked(edge)
		}
	}
	for _, target := range targets {
		g.addEdgeLocked(Edge{
			From: from,
			To:   resourceKey(target.Kind, target.Namespace, target.Name),
			Type: target.Type,
		})
	}
}

func (g *ResourceGraph) addEdgeLocked(edge Edge) {
	if edge.From == edge.To {
		return
	}
	from, to := edge.From, edge.To
	edgeType := edge.Type
	key := edgeKey(from, to, edgeType)
	if _, exists := g.edges[key]; exists {
		return
	}
	pair := relationshipKey(from, to)
	if g.edgeCounts[pair] == 0 {
		if g.dependencies[from] == nil {
			g.dependencies[from] = make(map[string]bool)
		}
		g.dependencies[from][to] = true
		if g.dependents[to] == nil {
			g.dependents[to] = make(map[string]bool)
		}
		g.dependents[to][from] = true
	}
	g.edgeCounts[pair]++
	g.edges[key] = edge
}

func (g *ResourceGraph) RemoveNode(kind, namespace, name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := resourceKey(kind, namespace, name)
	g.removeNodeLocked(key)
}

// RemoveEdgesFrom removes every outgoing edge of the given type from the node,
// leaving the node and its other edges intact. Resources rebuild their
// outgoing edges on update; without this, replaced dependencies would leave
// stale edges behind once RemoveNode is too destructive (it would also drop
// edges contributed by other nodes).
func (g *ResourceGraph) RemoveEdgesFrom(
	kind, namespace, name, edgeType string,
) {
	g.mu.Lock()
	defer g.mu.Unlock()
	from := resourceKey(kind, namespace, name)
	for _, edge := range g.edges {
		if edge.From == from && edge.Type == edgeType {
			g.removeEdgeLocked(edge)
		}
	}
}
