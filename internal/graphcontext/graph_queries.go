package graphcontext

import "sort"

// A malformed or unexpectedly broad topology must not turn one diagnosis into
// an unbounded memory/CPU operation. The limit is deliberately high enough for
// ordinary clusters while keeping worst-case traversal bounded.
const maxGraphTraversalNodes = 10000

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
		if len(visited) >= maxGraphTraversalNodes {
			break
		}
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
		if len(visited) >= maxGraphTraversalNodes {
			break
		}
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
