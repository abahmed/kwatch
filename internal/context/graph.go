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

type ResourceGraph struct {
	mu           sync.RWMutex
	dependents   map[string]map[string]bool
	dependencies map[string]map[string]bool
	edges        map[string]Edge
}

func NewResourceGraph() *ResourceGraph {
	return &ResourceGraph{
		dependents:   make(map[string]map[string]bool),
		dependencies: make(map[string]map[string]bool),
		edges:        make(map[string]Edge),
	}
}

func resourceKey(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

func edgeKey(from, to string) string {
	return from + "\x00" + to
}

func (g *ResourceGraph) AddEdge(fromKind, fromNS, fromName, toKind, toNS, toName, edgeType string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	from := resourceKey(fromKind, fromNS, fromName)
	to := resourceKey(toKind, toNS, toName)
	if from == to {
		return
	}
	if g.dependencies[from] == nil {
		g.dependencies[from] = make(map[string]bool)
	}
	g.dependencies[from][to] = true
	if g.dependents[to] == nil {
		g.dependents[to] = make(map[string]bool)
	}
	g.dependents[to][from] = true
	g.edges[edgeKey(from, to)] = Edge{From: from, To: to, Type: edgeType}
}

func (g *ResourceGraph) RemoveNode(kind, namespace, name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := resourceKey(kind, namespace, name)
	for dep := range g.dependents[key] {
		delete(g.dependencies[dep], key)
		delete(g.edges, edgeKey(dep, key))
	}
	for dep := range g.dependencies[key] {
		delete(g.dependents[dep], key)
		delete(g.edges, edgeKey(key, dep))
	}
	delete(g.dependencies, key)
	delete(g.dependents, key)
}

// RemoveEdgesFrom removes every outgoing edge of the given type from the node,
// leaving the node and its other edges intact. Resources rebuild their
// outgoing edges on update; without this, replaced dependencies would leave
// stale edges behind once RemoveNode is too destructive (it would also drop
// edges contributed by other nodes).
func (g *ResourceGraph) RemoveEdgesFrom(kind, namespace, name, edgeType string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	from := resourceKey(kind, namespace, name)
	if froms, ok := g.dependencies[from]; ok {
		for to := range froms {
			if e, ok := g.edges[edgeKey(from, to)]; ok && e.Type == edgeType {
				delete(g.edges, edgeKey(from, to))
				delete(froms, to)
				if toDeps, ok := g.dependents[to]; ok {
					delete(toDeps, from)
				}
			}
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
func (g *ResourceGraph) TraverseDependents(kind, namespace, name string) []string {
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
func (g *ResourceGraph) TraverseDependencies(kind, namespace, name string) []string {
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

func (g *ResourceGraph) DependentsByType(kind, namespace, name, depKind string) []string {
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
		return out[i].To < out[j].To
	})
	return out
}

func (g *ResourceGraph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dependencies = make(map[string]map[string]bool)
	g.dependents = make(map[string]map[string]bool)
	g.edges = make(map[string]Edge)
}

// Prune removes all nodes of the given kind whose key is not present in the
// active set. The active set should contain resource keys in "kind/ns/name"
// format. This is a mark-and-sweep for a specific resource type.
func (g *ResourceGraph) Prune(kind string, active map[string]bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	prefix := kind + "/"
	for key := range g.dependencies {
		if strings.HasPrefix(key, prefix) && !active[key] {
			for dep := range g.dependencies[key] {
				delete(g.dependents[dep], key)
				delete(g.edges, edgeKey(key, dep))
			}
			delete(g.dependencies, key)
		}
	}
	for key := range g.dependents {
		if strings.HasPrefix(key, prefix) && !active[key] {
			for dep := range g.dependents[key] {
				delete(g.dependencies[dep], key)
				delete(g.edges, edgeKey(dep, key))
			}
			delete(g.dependents, key)
		}
	}
}
