package context

import "strings"

// ReplaceWith atomically replaces this graph's contents with a consistent
// snapshot of next. Callers build next off to the side and only publish it
// after every required cache lookup succeeds, so a failed rebuild never
// empties the live graph.
func (g *ResourceGraph) ReplaceWith(next *ResourceGraph) {
	if next == nil || g == next {
		return
	}

	next.mu.RLock()
	dependencies := cloneAdjacency(next.dependencies)
	dependents := cloneAdjacency(next.dependents)
	edges := cloneEdges(next.edges)
	edgeCounts := cloneEdgeCounts(next.edgeCounts)
	next.mu.RUnlock()

	g.mu.Lock()
	defer g.mu.Unlock()
	g.dependencies = dependencies
	g.dependents = dependents
	g.edges = edges
	g.edgeCounts = edgeCounts
}

func (g *ResourceGraph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dependencies = make(map[string]map[string]bool)
	g.dependents = make(map[string]map[string]bool)
	g.edges = make(map[string]Edge)
	g.edgeCounts = make(map[string]int)
}

// Prune removes all nodes of the given kind whose key is not present in the
// active set. The active set should contain resource keys in "kind/ns/name"
// format. This is a mark-and-sweep for a specific resource type.
func (g *ResourceGraph) Prune(kind string, active map[string]bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	prefix := kind + "/"
	stale := make(map[string]bool)
	for key := range g.dependencies {
		if strings.HasPrefix(key, prefix) && !active[key] {
			stale[key] = true
		}
	}
	for key := range g.dependents {
		if strings.HasPrefix(key, prefix) && !active[key] {
			stale[key] = true
		}
	}
	for key := range stale {
		g.removeNodeLocked(key)
	}
}

func (g *ResourceGraph) removeNodeLocked(node string) {
	for _, edge := range g.edges {
		if edge.From == node || edge.To == node {
			g.removeEdgeLocked(edge)
		}
	}
	delete(g.dependencies, node)
	delete(g.dependents, node)
}

func (g *ResourceGraph) removeEdgeLocked(edge Edge) {
	delete(g.edges, edgeKey(edge.From, edge.To, edge.Type))
	pair := relationshipKey(edge.From, edge.To)
	g.edgeCounts[pair]--
	if g.edgeCounts[pair] > 0 {
		return
	}
	delete(g.edgeCounts, pair)
	if dependencies := g.dependencies[edge.From]; dependencies != nil {
		delete(dependencies, edge.To)
		if len(dependencies) == 0 {
			delete(g.dependencies, edge.From)
		}
	}
	if dependents := g.dependents[edge.To]; dependents != nil {
		delete(dependents, edge.From)
		if len(dependents) == 0 {
			delete(g.dependents, edge.To)
		}
	}
}

func cloneAdjacency(in map[string]map[string]bool) map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(in))
	for node, links := range in {
		out[node] = make(map[string]bool, len(links))
		for link, present := range links {
			out[node][link] = present
		}
	}
	return out
}

func cloneEdges(in map[string]Edge) map[string]Edge {
	out := make(map[string]Edge, len(in))
	for key, edge := range in {
		out[key] = edge
	}
	return out
}

func cloneEdgeCounts(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for key, count := range in {
		out[key] = count
	}
	return out
}
