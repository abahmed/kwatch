package pod

import (
	"testing"
	"time"
)

type oomTestClock struct {
	now time.Time
}

func (c *oomTestClock) Now() time.Time { return c.now }

func TestOOMTrackerUsesInjectedClock(t *testing.T) {
	clock := &oomTestClock{
		now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	tracker := newOOMTracker(2, time.Minute, clock)

	count, repeating := tracker.record("pod/container")
	if count != 1 || repeating {
		t.Fatalf("first OOM record = %d, %t", count, repeating)
	}
	clock.now = clock.now.Add(30 * time.Second)
	count, repeating = tracker.record("pod/container")
	if count != 2 || !repeating {
		t.Fatalf("second OOM record = %d, %t", count, repeating)
	}
	clock.now = clock.now.Add(62 * time.Second)
	count, repeating = tracker.record("pod/container")
	if count != 1 || repeating {
		t.Fatalf("expired OOM record = %d, %t", count, repeating)
	}
}
