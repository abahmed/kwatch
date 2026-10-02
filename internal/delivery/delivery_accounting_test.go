package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestDeliverOneDeadLettersOnlyWhenNothingIsLeft(t *testing.T) {
	failure := transport.Permanent(errors.New("rejected"))
	tests := []struct {
		name          string
		fallbackErr   error
		hasFallback   bool
		fallbackRoute []config.AlertRoute
		delivered     bool
		deadLetters   int64
	}{
		{name: "fallback succeeds", hasFallback: true,
			delivered: true, deadLetters: 0},
		{name: "fallback fails", hasFallback: true, fallbackErr: failure,
			delivered: false, deadLetters: 1},
		{name: "no fallback", delivered: false, deadLetters: 1},
		{name: "fallback routes exclude the job", hasFallback: true,
			fallbackRoute: []config.AlertRoute{
				{Namespaces: []string{"other"}},
			},
			delivered: false, deadLetters: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			primary := &errorRecorderProvider{name: "Primary", err: failure}
			fallback := &errorRecorderProvider{
				name: "Fallback", err: tc.fallbackErr,
			}
			fast := retryConfig{maxAttempts: 1, delay: time.Millisecond}
			entries := []providerEntry{{provider: primary, retry: fast}}
			if tc.hasFallback {
				entries[0].fallbackName = "Fallback"
				entries = append(entries, providerEntry{
					provider: fallback, retry: fast,
					routes: tc.fallbackRoute,
				})
			}
			am := managerWithEntries(entries)
			registry := metrics.DefaultRegistry()
			before := registry.DeliveryDeadLetters.Load()
			droppedBefore := registry.NotificationsDropped.Load()

			got := am.deliverOne(context.Background(),
				&managerEntries(am)[0], incidentJob("k", "shop"))

			after := registry.DeliveryDeadLetters.Load()
			assert.Equal(t, tc.delivered, got)
			assert.Equal(t, tc.deadLetters, after-before)
			assert.Equal(t, tc.deadLetters,
				registry.NotificationsDropped.Load()-droppedBefore,
				"a job is counted as dropped only when dead-lettered")
		})
	}
}

func TestDeliverOneSkipsFallbackAfterCancellation(t *testing.T) {
	primary := &errorRecorderProvider{
		name: "Primary", err: errors.New("cut short"),
	}
	fallback := &errorRecorderProvider{name: "Fallback"}
	fast := retryConfig{maxAttempts: 1, delay: time.Millisecond}
	am := managerWithEntries([]providerEntry{
		{provider: primary, retry: fast, fallbackName: "Fallback"},
		{provider: fallback, retry: fast},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()

	got := am.deliverOne(ctx, &managerEntries(am)[0],
		incidentJob("k", "shop"))

	assert.False(t, got)
	assert.Zero(t, fallback.callCount, "fallback skipped once cancelled")
	assert.Equal(t, int64(1),
		metrics.DefaultRegistry().DeliveryDeadLetters.Load()-before)
}

func TestPendingQueueOverflowIsCounted(t *testing.T) {
	am := managerWithEntries([]providerEntry{{provider: &fakeProvider{}}})
	for i := 0; i < channelCap; i++ {
		am.Notify("queued before start")
	}
	before := metrics.DefaultRegistry().DeliveryPendingDropped.Load()

	am.Notify("one too many")

	after := metrics.DefaultRegistry().DeliveryPendingDropped.Load()
	assert.Equal(t, int64(1), after-before)
	assert.Len(t, am.pending, channelCap)
}

func TestStopBeforeStartCountsPendingJobs(t *testing.T) {
	am := managerWithEntries([]providerEntry{{provider: &fakeProvider{}}})
	am.Notify("first")
	am.Notify("second")
	before := metrics.DefaultRegistry().DeliveryPendingDropped.Load()

	require.NoError(t, am.Stop(context.Background()))

	after := metrics.DefaultRegistry().DeliveryPendingDropped.Load()
	assert.Equal(t, int64(2), after-before)
	assert.Empty(t, am.pending)
}
