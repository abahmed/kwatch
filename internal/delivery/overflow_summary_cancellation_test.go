package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFlushOverflowSummaryRestoresWhenCancelledWhileWaiting(
	t *testing.T,
) {
	provider := &errorRecorderProvider{name: "Digest"}
	manager := newTestManager()
	manager.pacer.interval = time.Hour
	entry := &providerEntry{provider: provider}

	// Reserve the first slot so the summary must wait for the pacing interval.
	require.True(t, manager.waitForSendSlot(context.Background(), "Digest"))
	manager.addToOverflowSummary("Digest", incidentJob("Error", "default"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	manager.flushOverflowSummary(ctx, entry)

	manager.pacer.mu.Lock()
	state := manager.pacer.summaries["Digest"]
	manager.pacer.mu.Unlock()
	require.NotNil(t, state)
	require.Equal(t, 1, state.total)
	require.Zero(t, provider.callCount)
}
