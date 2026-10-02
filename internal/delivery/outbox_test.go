package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestOutboxResendsJobQueuedBeforeCrash(t *testing.T) {
	stuck := newScriptedProvider(func(ctx context.Context, _ int, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	})
	store := newMemOutbox()
	first := outboxManager(t, stuck, store)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		ended, end := context.WithCancel(context.Background())
		end()
		_ = first.Stop(ended)
	})
	require.NoError(t, first.Start(ctx))
	first.Notify("disk full")
	store.waitForRecords(t, 1)

	// The process dies here: only what reached the store survives.
	provider := newScriptedProvider(nil)
	second := outboxManager(t, provider, store.copy())
	require.NoError(t, second.Start(context.Background()))
	t.Cleanup(func() { shutdownManager(second) })

	assert.Equal(t, "disk full", provider.receive(t))
}

func TestOutboxForgetsAcceptedJob(t *testing.T) {
	provider := newScriptedProvider(nil)
	store := newMemOutbox()
	manager := outboxManager(t, provider, store)
	require.NoError(t, manager.Start(context.Background()))
	manager.Notify("node ready")
	require.Equal(t, "node ready", provider.receive(t))

	require.NoError(t, manager.Stop(context.Background()))

	assert.Zero(t, store.len(), "an accepted job is not resent")
}

func TestOutboxRemovesPermanentFailure(t *testing.T) {
	rejecting := newScriptedProvider(func(context.Context, int, string) error {
		return transport.Permanent(errors.New("rejected"))
	})
	store := newMemOutbox()
	manager := outboxManager(t, rejecting, store)
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()
	require.NoError(t, manager.Start(context.Background()))
	manager.Notify("bad payload")

	require.NoError(t, manager.Stop(context.Background()))

	assert.Equal(t, int64(1),
		metrics.DefaultRegistry().DeliveryDeadLetters.Load()-before)
	assert.Zero(t, store.len(), "a permanent failure is not retried")
}

func TestOutboxKeepsJobStopCouldNotSend(t *testing.T) {
	provider := newScriptedProvider(nil)
	store := newMemOutbox()
	manager := outboxManager(t, provider, store)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, manager.Start(ctx))
	stopWorkers(manager, cancel)
	manager.Notify("late")
	ended, end := context.WithCancel(context.Background())
	end()

	require.Error(t, manager.Stop(ended))

	assert.Equal(t, 1, store.len(), "the next session sends it")
}

func TestOutboxRestoresInOrderBeforeNewWork(t *testing.T) {
	now := time.Now()
	store := newMemOutbox(
		messageRecord(outboxID(2), "second", now),
		messageRecord(outboxID(1), "first", now),
		messageRecord(outboxID(3), "third", now),
	)
	provider := newScriptedProvider(nil)
	manager := outboxManager(t, provider, store)
	manager.Notify("new")
	require.NoError(t, manager.Start(context.Background()))
	t.Cleanup(func() { shutdownManager(manager) })

	got := []string{provider.receive(t), provider.receive(t),
		provider.receive(t), provider.receive(t)}

	assert.Equal(t, []string{"first", "second", "third", "new"}, got)
}

func TestOutboxRestoreDropsUnusableRecords(t *testing.T) {
	now := time.Now()
	stale := messageRecord(outboxID(1), "stale", now.Add(-2*outboxMaxAge))
	future := messageRecord(outboxID(2), "future", now)
	future.Version = outboxRecordVersion + 1
	gone := messageRecord(outboxID(3), "gone", now)
	gone.Provider = "removed-provider"
	store := newMemOutbox(stale, future, gone,
		messageRecord(outboxID(4), "kept", now))
	before := metrics.DefaultRegistry().OutboxDropped.Load()
	provider := newScriptedProvider(nil)
	manager := outboxManager(t, provider, store)

	require.NoError(t, manager.Start(context.Background()))

	assert.Equal(t, "kept", provider.receive(t))
	require.NoError(t, manager.Stop(context.Background()))
	assert.Equal(t, int64(3),
		metrics.DefaultRegistry().OutboxDropped.Load()-before)
	assert.Zero(t, store.len())
}

func TestOutboxDropsOldestOverBound(t *testing.T) {
	store := newMemOutbox()
	box := newOutbox(store, time.Now)
	before := metrics.DefaultRegistry().OutboxDropped.Load()

	for i := 0; i < outboxMaxEntries+5; i++ {
		box.add(deliverJob{kind: jobMessage, msg: "m"}, "slack")
	}
	box.flush()

	records, err := store.LoadOutbox()
	require.NoError(t, err)
	require.Len(t, records, outboxMaxEntries)
	assert.Equal(t, outboxID(6), records[0].ID, "the oldest are dropped")
	assert.Equal(t, int64(5),
		metrics.DefaultRegistry().OutboxDropped.Load()-before)
}

func TestOutboxRetriesFailedWrite(t *testing.T) {
	store := &failingOutbox{memOutbox: newMemOutbox(), failures: 1}
	box := newOutbox(store, time.Now)
	before := metrics.DefaultRegistry().OutboxWriteFailures.Load()
	box.add(deliverJob{kind: jobMessage, msg: "m"}, "slack")

	box.flush()
	require.Zero(t, store.len())
	box.flush()

	assert.Equal(t, 1, store.len(), "the failed batch is written later")
	assert.Equal(t, int64(1),
		metrics.DefaultRegistry().OutboxWriteFailures.Load()-before)
}

// failingOutbox fails its first writes, then behaves like memOutbox.
type failingOutbox struct {
	*memOutbox
	failures int
}

func (f *failingOutbox) WriteOutbox(
	puts []OutboxRecord, removes []string,
) error {
	if f.failures > 0 {
		f.failures--
		return errors.New("disk unavailable")
	}
	return f.memOutbox.WriteOutbox(puts, removes)
}
