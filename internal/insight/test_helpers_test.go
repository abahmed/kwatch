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
