package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestFlushDigestRestoresWhenContextCancelsWhileWaiting(t *testing.T) {
	provider := &errorRecorderProvider{name: "Digest"}
	manager := newTestManager()
	manager.pacer.interval = time.Hour
	entry := &providerEntry{provider: provider}

	// Reserve the first slot so the digest must wait for the pacing interval.
	require.True(t, manager.waitForSendSlot(context.Background(), "Digest"))
	manager.digestAdd("Digest", deliverJob{
		kind: jobEvent,
		ev:   &event.Event{Reason: "Error"},
		inc:  &model.Incident{},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	manager.flushDigest(ctx, entry)

	manager.pacer.mu.Lock()
	state := manager.pacer.digests["Digest"]
	manager.pacer.mu.Unlock()
	require.NotNil(t, state)
	require.Equal(t, 1, state.total)
	require.Zero(t, provider.callCount)
}
