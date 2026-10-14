package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A provider that panics on one message must not stop the process or the
// other jobs: the poison job is given up after maxUnfinishedSends.
func TestProviderPanicDeadLettersPoisonJobOnly(t *testing.T) {
	now := time.Now()
	store := newMemOutbox(
		messageRecord(outboxID(1), "poison", now),
		messageRecord(outboxID(2), "good", now),
	)
	poisonCalls := 0
	provider := newScriptedProvider(func(
		_ context.Context, _ int, text string,
	) error {
		if text == "poison" {
			poisonCalls++
			panic("boom: " + text)
		}
		return nil
	})
	manager := outboxManager(t, provider, store)
	require.NoError(t, manager.Start(context.Background()))
	assert.Equal(t, "good", provider.receive(t))
	require.NoError(t, manager.Stop(context.Background()))

	assert.Equal(t, maxUnfinishedSends, poisonCalls)
	assert.Zero(t, store.len(), "the poison job leaves the outbox")
}

// A job that already began its sends maxUnfinishedSends times before a
// restart is dead-lettered without another send.
func TestRestoredJobAtAttemptBoundIsNotSent(t *testing.T) {
	record := messageRecord(outboxID(1), "poison", time.Now())
	record.Attempts = maxUnfinishedSends
	store := newMemOutbox(record)
	provider := newScriptedProvider(nil)
	manager := outboxManager(t, provider, store)
	require.NoError(t, manager.Start(context.Background()))
	require.NoError(t, manager.Stop(context.Background()))

	assert.Zero(t, sentCalls(provider))
	assert.Zero(t, store.len())
}
