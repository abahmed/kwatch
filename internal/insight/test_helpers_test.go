package insight

import (
	"github.com/abahmed/kwatch/internal/clock"
	context "github.com/abahmed/kwatch/internal/graphcontext"
)

func newTestEngine(
	graph *context.ResourceGraph,
	tracker *context.ChangeTracker,
) *Engine {
	return NewEngineWithClock(graph, tracker, clock.RealClock{})
}

func newTestChangeTracker(capacity int) *context.ChangeTracker {
	return context.NewChangeTrackerWithClock(capacity, clock.RealClock{})
}

// withActiveNode marks the named nodes as having active incidents, which is
// what lets insight name a node as the cause.
func withActiveNode(e *Engine, nodes ...string) *Engine {
	active := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		active[node] = true
	}
	e.activeChecker = func(kind, _, name string) bool {
		return kind == "node" && active[name]
	}
	return e
}
