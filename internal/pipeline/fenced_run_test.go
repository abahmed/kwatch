package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Once the store is claimed by a newer leader, Run returns instead of
// going on to send messages for state this process no longer owns.
func TestRunReturnsOnceTheStoreIsFenced(t *testing.T) {
	progress := make(chan struct{}, 8)
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0),
		ch: make(chan time.Time)}
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Store = &memStore{}
		d.Progress = func() { progress <- struct{}{} }
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startRun(ctx, e)
	e.Submit(ctx, inventory.Observation{Kind: inventory.Observed})
	waitFor(t, progress)

	e.storage.incidents.stopFenced()
	e.Submit(ctx, inventory.Observation{Kind: inventory.Observed})

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrFenced)
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Run kept going after the store was fenced")
	}
}
