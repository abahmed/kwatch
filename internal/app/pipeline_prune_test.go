package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Baseline expiry belongs to the pipeline's history writer, which also
// saves baselines; the model pruner must not delete them on its own
// goroutine.
func TestPruneOnceLeavesBaselinesToTheHistoryWriter(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := inventory.CoreID(kube.KindDeployment, "apps", "old")
	model.Baselines().Add(id, inventory.MetricReadySeconds, 1, at)

	pruneOnce(model, at.Add(inventory.BaselineTTL+time.Hour))

	_, ok := model.Baselines().Stat(id, inventory.MetricReadySeconds)
	require.True(t, ok)
}

// fakeTimerClock records the timers the pipeline asks for.
type fakeTimerClock struct{ fired chan time.Time }

func (fakeTimerClock) Now() time.Time { return time.Unix(0, 0) }

func (c fakeTimerClock) After(time.Duration) <-chan time.Time {
	return c.fired
}

func TestPipelineClockUsesInjectedTimers(t *testing.T) {
	injected := fakeTimerClock{fired: make(chan time.Time)}
	c := pipelineClock{now: injected}
	if c.After(time.Hour) != (<-chan time.Time)(injected.fired) {
		t.Fatal("After must come from the injected clock")
	}
	// A clock without timers still gets a working channel.
	fallback := pipelineClock{now: clock.Func(time.Now)}
	if fallback.After(0) == nil {
		t.Fatal("fallback timer missing")
	}
}
