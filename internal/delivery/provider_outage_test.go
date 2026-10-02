package delivery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestManagerDeliversLatestStateOnceAfterThirtyMinuteOutage(t *testing.T) {
	clk := newFakeClock()
	recovered := clk.Now().Add(30 * time.Minute)
	provider := newMessageProvider(
		func(int, notification.Message) error {
			if clk.Now().Before(recovered) {
				return errors.New("dial tcp: connection refused")
			}
			return nil
		})
	store := newMemOutbox()
	manager := fakeClockManager(t, clk, provider, store)
	// The first wait holds until every revision below is queued behind
	// the job the worker keeps.
	firstWait, gate := make(chan struct{}), make(chan struct{})
	var once sync.Once
	manager.sleepFn = func(ctx context.Context, d time.Duration) bool {
		once.Do(func() {
			close(firstWait)
			<-gate
		})
		return advancingSleep(clk)(ctx, d)
	}
	registry := metrics.DefaultRegistry()
	deadBefore := registry.DeliveryDeadLetters.Load()
	startManager(t, manager)

	openA := revision("a", 1, "🔴 a is down")
	manager.NotifyIncident(openA)
	<-firstWait
	manager.NotifyIncident(revision("a", 2, "🔴 a is still down"))
	manager.NotifyIncident(revision("b", 1, "🔴 b is slow"))
	manager.NotifyIncident(revision("b", 2, "🔴 b is down"))
	manager.NotifyIncident(resolvedRevision("a", 3, "✅ a recovered",
		&openA))
	manager.NotifyIncident(revision("c", 1, "🔴 c is down"))
	close(gate)

	a, b, c := provider.receive(t), provider.receive(t), provider.receive(t)
	assert.Equal(t, 3, a.Revision, "a is sent once, as its resolve")
	assert.Contains(t, a.Note, "✅ a recovered")
	assert.Contains(t, a.Note, "🔴 a is down",
		"the lost announcement is combined into the resolve")
	assert.Equal(t, 2, b.Revision, "b is sent once, as its newest state")
	assert.Equal(t, 1, c.Revision)
	assert.False(t, clk.Now().Before(recovered))
	assert.Equal(t, deadBefore, registry.DeliveryDeadLetters.Load(),
		"nothing was dead-lettered")
	store.waitForRecords(t, 0)
	select {
	case extra := <-provider.sent:
		t.Fatalf("unexpected extra delivery %q", extra.Key)
	default:
	}
}

func TestManagerDeadLettersRejectionAtOnce(t *testing.T) {
	clk := newFakeClock()
	provider := newMessageProvider(func(int, notification.Message) error {
		return transport.Permanent(errors.New("invalid token"))
	})
	store := newMemOutbox()
	manager := fakeClockManager(t, clk, provider, store)
	before := metrics.DefaultRegistry().DeliveryDeadLetters.Load()
	require.NoError(t, manager.Start(context.Background()))

	manager.NotifyIncident(revision("a", 1, "🔴 a is down"))
	require.NoError(t, manager.Stop(context.Background()))

	assert.Zero(t, store.len())
	assert.Equal(t, 1, provider.callCount(), "a rejection is not retried")
	assert.Equal(t, before+1,
		metrics.DefaultRegistry().DeliveryDeadLetters.Load())
}

func TestWorkOneGivesUpJobOlderThanOutboxMaxAge(t *testing.T) {
	clk := newFakeClock()
	provider := newMessageProvider(func(int, notification.Message) error {
		return errors.New("HTTP 503")
	})
	manager := fakeClockManager(t, clk, provider, nil)
	entry := manager.generation.entries["slack"]
	job := deliverJob{
		kind: jobIncident, target: "slack", queued: clk.Now(),
		incident: &notification.Message{Key: "a", Revision: 2},
	}
	registry := metrics.DefaultRegistry()
	before := registry.DeliveryDeadLetters.Load()

	require.True(t, manager.workOne(context.Background(), &entry, job))

	assert.Greater(t, clk.Now().Sub(job.queued), outboxMaxAge)
	assert.Equal(t, before+1, registry.DeliveryDeadLetters.Load())
	assert.Equal(t, "expired", deadLetterReason(errJobExpired))
}
