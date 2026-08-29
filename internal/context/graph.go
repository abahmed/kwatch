package context

import (
	"sort"
	"strings"
	"sync"
)

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

func (g *ResourceGraph) DependenciesOf(kind, namespace, name string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	key := resourceKey(kind, namespace, name)
	deps := g.dependencies[key]
	out := make([]string, 0, len(deps))
	for d := range deps {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// TraverseDependents returns every node reachable from the given node by
// following dependents edges (BFS), including indirect ("transitive")
// dependents. The starting node itself is never included.
func (g *ResourceGraph) TraverseDependents(
	kind, namespace, name string,
) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	root := resourceKey(kind, namespace, name)
	visited := map[string]bool{root: true}
	queue := []string{root}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for dep := range g.dependents[cur] {
			if visited[dep] {
				continue
			}
			visited[dep] = true
			out = append(out, dep)
			queue = append(queue, dep)
		}
	}
	sort.Strings(out)
	return out
}

// TraverseDependencies returns every node reachable from the given node by
// following dependency edges (BFS), including indirect ("transitive")
// dependencies. This walks backwards from an incident toward its root causes.
func (g *ResourceGraph) TraverseDependencies(
	kind, namespace, name string,
) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	root := resourceKey(kind, namespace, name)
	visited := map[string]bool{root: true}
	queue := []string{root}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for dep := range g.dependencies[cur] {
			if visited[dep] {
				continue
			}
			visited[dep] = true
			out = append(out, dep)
			queue = append(queue, dep)
		}
	}
	sort.Strings(out)
	return out
}

func (g *ResourceGraph) DependentsOf(kind, namespace, name string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	key := resourceKey(kind, namespace, name)
	deps := g.dependents[key]
	out := make([]string, 0, len(deps))
	for d := range deps {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func (g *ResourceGraph) DependentsByType(
	kind, namespace, name, depKind string,
) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	key := resourceKey(kind, namespace, name)
	deps := g.dependents[key]
	prefix := depKind + "/"
	var out []string
	for d := range deps {
		if len(d) > len(prefix) && d[:len(prefix)] == prefix {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

func (g *ResourceGraph) Edges() []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]Edge, 0, len(g.edges))
	for _, e := range g.edges {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Type < out[j].Type
	})
	return out
}

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
