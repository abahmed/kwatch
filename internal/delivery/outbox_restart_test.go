package delivery

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOutboxRestartResendsQueuedMessagesOnce crashes a session with two
// queued messages, restarts from what reached the store and checks each
// message is sent exactly once: by the first restart, and never again by
// a second restart from the store the first one left behind.
func TestOutboxRestartResendsQueuedMessagesOnce(t *testing.T) {
	now := time.Now()
	crashed := newMemOutbox(
		messageRecord(outboxID(1), "disk full", now),
		messageRecord(outboxID(2), "node lost", now),
	)

	provider := newScriptedProvider(nil)
	first := outboxManager(t, provider, crashed)
	require.NoError(t, first.Start(context.Background()))
	got := []string{provider.receive(t), provider.receive(t)}
	require.NoError(t, first.Stop(context.Background()))

	sort.Strings(got)
	assert.Equal(t, []string{"disk full", "node lost"}, got)
	assert.Zero(t, crashed.len(), "sent jobs leave the outbox")

	again := newScriptedProvider(nil)
	second := outboxManager(t, again, crashed.copy())
	require.NoError(t, second.Start(context.Background()))
	require.NoError(t, second.Stop(context.Background()))

	assert.Equal(t, 2, sentCalls(provider), "each message sent once")
	assert.Zero(t, sentCalls(again), "a second restart resends nothing")
}

func sentCalls(p *scriptedProvider) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}
