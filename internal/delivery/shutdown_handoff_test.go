package delivery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/metrics"
)

func TestStopLetsTheRequestInFlightFinish(t *testing.T) {
	inFlight := make(chan struct{})
	release := make(chan struct{})
	provider := newScriptedProvider(
		func(ctx context.Context, call int, _ string) error {
			if call > 1 {
				return nil
			}
			// The first send is still running when the session ends.
			close(inFlight)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	manager := outboxManager(t, provider, nil)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, manager.Start(ctx))
	manager.Notify("in flight")
	<-inFlight
	cancel()
	close(release)
	stopWorkers(manager, cancel)
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()

	require.NoError(t, manager.Stop(context.Background()))

	assert.Equal(t, "in flight", provider.receive(t))
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	assert.Equal(t, 1, calls, "the request is not sent twice")
	assert.Equal(t, before,
		metrics.DefaultRegistry().DeliveryDeadLetters.Load())
}

func TestStopCutsTheRequestInFlightAtItsDeadline(t *testing.T) {
	inFlight := make(chan struct{})
	provider := newScriptedProvider(
		func(ctx context.Context, _ int, _ string) error {
			select {
			case <-inFlight:
			default:
				close(inFlight)
			}
			<-ctx.Done()
			return ctx.Err()
		})
	store := newMemOutbox()
	manager := outboxManager(t, provider, store)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, manager.Start(ctx))
	manager.Notify("in flight")
	<-inFlight
	cancel()
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()
	ended, end := context.WithCancel(context.Background())
	end()

	require.Error(t, manager.Stop(ended))
	stopWorkers(manager, cancel)

	assert.Equal(t, before,
		metrics.DefaultRegistry().DeliveryDeadLetters.Load(),
		"a job the outbox keeps for the next session is no dead letter")
	assert.Equal(t, 1, store.len(), "the outbox keeps it for next session")
}
