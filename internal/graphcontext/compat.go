package graphcontext

import (
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// NewChangeTracker preserves the historical function-clock constructor for
// standalone and embedded callers. Application composition uses
// NewChangeTrackerWithClock.
func NewChangeTracker(
	capacity int,
	clocks ...func() time.Time,
) *ChangeTracker {
	return NewChangeTrackerWithClock(
		capacity, clock.Func(clock.From(clocks)),
	)
}
