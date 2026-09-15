package insight

import (
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	context "github.com/abahmed/kwatch/internal/graphcontext"
)

// NewEngine preserves the historical function-clock constructor for
// standalone callers. Application composition uses NewEngineWithClock.
func NewEngine(
	graph *context.ResourceGraph,
	tracker *context.ChangeTracker,
	clocks ...func() time.Time,
) *Engine {
	return NewEngineWithClock(
		graph, tracker, clock.Func(clock.From(clocks)),
	)
}
