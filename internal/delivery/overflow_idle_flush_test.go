package delivery

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

func summaryManager(
	t *testing.T, fail func(int, notification.Message) error,
) (*Manager, *messageProvider) {
	t.Helper()
	provider := newMessageProvider(fail)
	manager := fakeClockManager(t, newFakeClock(), provider, nil)
	manager.workers.progressEvery = 5 * time.Millisecond
	return manager, provider
}

// An idle worker retries a summary that a failed flush restored; no job
// has to complete to trigger the flush.
func TestIdleWorkerRetriesRestoredOverflowSummary(t *testing.T) {
	manager, provider := summaryManager(t,
		func(call int, _ notification.Message) error {
			if call == 1 {
				return errors.New("timeout")
			}
			return nil
		})
	manager.addToOverflowSummary("slack", incidentJob("k", "ns"))
	startManager(t, manager)

	got := provider.receive(t)

	assert.Contains(t, got.Note, "not sent one by one")
	assert.GreaterOrEqual(t, provider.callCount(), 2)
}

// Shutdown flushes a pending summary of a provider whose queue is empty.
func TestShutdownFlushesPendingOverflowSummary(t *testing.T) {
	manager, provider := summaryManager(t, nil)
	manager.workers.progressEvery = time.Hour
	startManager(t, manager)
	manager.addToOverflowSummary("slack", incidentJob("k", "ns"))

	require.NoError(t, manager.Stop(t.Context()))

	require.Equal(t, 1, provider.callCount())
	assert.True(t, strings.Contains(provider.receive(t).Note, "k failed"))
}
