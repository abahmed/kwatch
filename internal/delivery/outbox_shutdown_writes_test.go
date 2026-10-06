package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Removals made while delivery drains after its context ended reach the
// disk at once, not only in the last write: a kill during the drain must
// not send the delivered jobs again next session.
func TestOutboxWritesRemovalsAfterContextEnded(t *testing.T) {
	store := newMemOutbox()
	box := newOutbox(store, newFakeClock().Now)
	ctx, cancel := context.WithCancel(context.Background())
	box.start(ctx)
	defer func() { _ = box.close(outboxFinalTimeout) }()

	id := box.add(deliverJob{kind: jobMessage, msg: "hello"}, "slack")
	store.waitForRecords(t, 1)
	cancel()
	box.remove(id)

	store.waitForRecords(t, 0)
	require.Zero(t, store.len())
}

func TestOutboxDrainGraceIsThirtySeconds(t *testing.T) {
	require.Equal(t, 30*time.Second, outboxDrainGrace)
	require.Equal(t, outboxDrainGrace,
		newOutbox(newMemOutbox(), newFakeClock().Now).graceAfterContext())
}

// The writer outlives its context only by the drain grace: after that it
// stops on its own, so a stuck delivery cannot keep it for ever.
func TestOutboxWriterStopsAfterTheDrainGrace(t *testing.T) {
	box := newOutbox(newMemOutbox(), newFakeClock().Now)
	box.drainGrace = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	box.start(ctx)

	select {
	case <-box.done:
		t.Fatal("the writer stopped before its context ended")
	default:
	}
	cancel()

	select {
	case <-box.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the writer outlived its drain grace")
	}
}
