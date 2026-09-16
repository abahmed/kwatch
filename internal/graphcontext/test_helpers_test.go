package graphcontext

import "github.com/abahmed/kwatch/internal/clock"

func newTestChangeTracker(capacity int) *ChangeTracker {
	return NewChangeTrackerWithClock(capacity, clock.RealClock{})
}
