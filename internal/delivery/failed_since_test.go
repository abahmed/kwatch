package delivery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// failingManager is a manager whose one provider refuses every send,
// on a clock the test moves.
func failingManager(t *testing.T) (*Manager, *providerEntry, *fakeClock) {
	t.Helper()
	clock := newFakeClock()
	provider := &errorRecorderProvider{name: "slack",
		err: transport.Permanent(errors.New("bad token"))}
	am := newTestManager()
	am.now = clock.Now
	setManagerEntries(am, []providerEntry{{provider: provider,
		retry: retryConfig{maxAttempts: 1, delay: time.Millisecond}}})
	entry := managerEntries(am)[0]
	return am, &entry, clock
}

func TestManagerFailedSinceCountsSummariesAndMessages(t *testing.T) {
	am, entry, clock := failingManager(t)
	ctx := context.Background()
	for _, key := range []string{
		notification.SummaryKeyPrefix + "start",
		notification.RollupKeyPrefix + "r",
		notification.DigestKeyPrefix + "d",
	} {
		job := deliverJob{kind: jobIncident,
			incident: &notification.Message{Key: key, Opens: true}}
		_ = am.dispatch(ctx, entry, job, deliverOpts{})
	}
	_ = am.dispatch(ctx, entry, deliverJob{kind: jobMessage, msg: "hi"},
		deliverOpts{})
	since := clock.Now().Add(-time.Hour)
	if got := am.FailedSince(since); got != 4 {
		t.Errorf("FailedSince = %d, want 4", got)
	}
}

func TestManagerFailedSinceCountsPastTheTrailBound(t *testing.T) {
	am, entry, clock := failingManager(t)
	ctx := context.Background()
	sends := maxTrailAttempts + 9
	for range sends {
		_ = am.dispatch(ctx, entry, trailJob("inc-1"), deliverOpts{})
	}
	if got := am.FailedSince(clock.Now().Add(-time.Hour)); got != sends {
		t.Errorf("FailedSince = %d, want %d", got, sends)
	}
	if got := len(am.SendTrail("inc-1")); got != maxTrailAttempts {
		t.Errorf("trail kept %d", got)
	}
}

func TestManagerFailedSinceSkipsAcceptedSends(t *testing.T) {
	am, entry, clock := failingManager(t)
	entry.provider.(*errorRecorderProvider).err = nil
	_ = am.dispatch(context.Background(), entry, trailJob("inc-1"),
		deliverOpts{})
	if got := am.FailedSince(clock.Now().Add(-time.Hour)); got != 0 {
		t.Errorf("FailedSince = %d, want 0", got)
	}
}

func TestFailCounterDropsFailuresOlderThanTheWindow(t *testing.T) {
	var c failCounter
	start := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	c.add(start)
	c.add(start.Add(10 * time.Minute))
	c.add(start.Add(30 * time.Minute))
	now := start.Add(30 * time.Minute)
	if got := c.since(now.Add(-time.Hour), now); got != 3 {
		t.Errorf("since(-1h) = %d, want 3", got)
	}
	if got := c.since(start.Add(10*time.Minute), now); got != 2 {
		t.Errorf("since(10:10) = %d, want 2", got)
	}
	now = start.Add(70 * time.Minute)
	if got := c.since(now.Add(-time.Hour), now); got != 2 {
		t.Errorf("an hour on: %d, want 2", got)
	}
	if got := c.since(time.Time{}, now); got != 2 {
		t.Errorf("since(zero) = %d, want 2", got)
	}
	// The bucket of 10:00 is reused 61 minutes on; the old count goes.
	c.add(start.Add(time.Duration(failBuckets) * time.Minute))
	now = start.Add(time.Duration(failBuckets) * time.Minute)
	if got := c.since(now.Add(-time.Hour), now); got != 3 {
		t.Errorf("after reuse: %d, want 3", got)
	}
	if got := c.since(now, now.Add(-2*time.Hour)); got != 0 {
		t.Errorf("window in the future: %d, want 0", got)
	}
}

func TestFailCounterCountsConcurrentFailures(t *testing.T) {
	var c failCounter
	at := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 500 {
				c.add(at.Add(time.Duration(i*j%60) * time.Second))
				_ = c.since(at, at)
			}
		})
	}
	wg.Wait()
	if got := c.since(at, at.Add(time.Minute)); got != 8*500 {
		t.Errorf("since = %d, want %d", got, 8*500)
	}
}

func TestManagerFailedSinceWithoutFailuresIsZero(t *testing.T) {
	if got := newTestManager().FailedSince(time.Time{}); got != 0 {
		t.Errorf("FailedSince = %d, want 0", got)
	}
	if got := (&Manager{}).FailedSince(time.Time{}); got != 0 {
		t.Errorf("zero manager: FailedSince = %d, want 0", got)
	}
}
