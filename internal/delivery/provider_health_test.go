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
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func TestProviderHealthReportsLastOutcomeWithBoundedReasons(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"outage", errors.New("dial tcp 10.0.0.1: refused"),
			reasonUnavailable},
		{"rejection", transport.Permanent(errors.New("bad token")),
			reasonRejected},
		{"rate limit", &ratelimit.Error{StatusCode: 429}, reasonRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock()
			provider := newMessageProvider(
				func(int, notification.Message) error { return tc.err })
			manager := fakeClockManager(t, clk, provider, nil)
			entry := manager.generation.entries["slack"]
			drainHealthEvents(manager)

			manager.deliver(context.Background(), &entry,
				deliverJob{kind: jobMessage, msg: "x"})

			assertHealthEvent(t, manager)
			assert.Equal(t, []ProviderHealth{{"slack", tc.want}},
				manager.ProviderHealth())

			provider.fail = nil
			manager.pacer = sendPacer{}
			manager.deliver(context.Background(), &entry,
				deliverJob{kind: jobMessage, msg: "x"})

			assertHealthEvent(t, manager)
			assert.Equal(t, []ProviderHealth{{"slack", ""}},
				manager.ProviderHealth(), "success clears the reason")
		})
	}
}

func drainHealthEvents(manager *Manager) {
	select {
	case <-manager.ProviderHealthEvents():
	default:
	}
}

func assertHealthEvent(t *testing.T, manager *Manager) {
	t.Helper()
	select {
	case <-manager.ProviderHealthEvents():
	default:
		t.Fatal("no provider health event")
	}
}

func TestDeliveryPublishesQueueAndOutboxDepth(t *testing.T) {
	clk := newFakeClock()
	inFlight, release := make(chan struct{}), make(chan struct{})
	provider := newMessageProvider(func(call int, _ notification.Message) error {
		if call == 1 {
			close(inFlight)
			<-release
		}
		return nil
	})
	store := newMemOutbox()
	manager := fakeClockManager(t, clk, provider, store)
	startManager(t, manager)
	defer close(release)
	delivery := &metrics.DefaultRegistry().Delivery

	manager.NotifyIncident(revision("a", 1, "🔴 a"))
	<-inFlight
	for _, key := range []string{"b", "c", "d"} {
		manager.NotifyIncident(revision(key, 1, "🔴 "+key))
	}

	assert.Equal(t, int64(3), delivery.QueueDepth("slack"))
	assert.Equal(t, int64(4), delivery.OutboxDepth.Load())
}

func TestStreamedBacklogDeliversEveryRestoredJob(t *testing.T) {
	provider := newScriptedProvider(nil)
	provider.sent = make(chan string, outboxMaxEntries)
	queued := time.Now()
	records := make([]OutboxRecord, 0, outboxMaxEntries)
	for i := 1; i <= outboxMaxEntries; i++ {
		records = append(records,
			messageRecord(outboxID(uint64(i)), "restored", queued))
	}
	store := newMemOutbox(records...)
	manager := outboxManager(t, provider, store)
	manager.sleepFn = func(ctx context.Context, _ time.Duration) bool {
		return ctx.Err() == nil
	}
	saturated := metrics.DefaultRegistry().DeliveryQueueSaturated.Load()

	require.NoError(t, manager.Start(context.Background()))
	for i := 0; i < outboxMaxEntries; i++ {
		provider.receive(t)
	}
	require.NoError(t, manager.Stop(context.Background()))

	assert.Equal(t, saturated,
		metrics.DefaultRegistry().DeliveryQueueSaturated.Load(),
		"restored jobs stream in instead of overflowing the queue")
	assert.Zero(t, store.len())
}

func TestRestoredJobForRenamedProviderIsRetargeted(t *testing.T) {
	provider := newScriptedProvider(nil)
	queued := time.Now()
	renamed := messageRecord(outboxID(1), "renamed", queued)
	renamed.Provider = "incident.io"
	gone := messageRecord(outboxID(2), "gone", queued)
	gone.Provider = "removed-provider"
	store := newMemOutbox(renamed, gone)
	manager := newManagerFor(t, "incidentio", provider)
	require.NoError(t, manager.AttachOutbox(store))
	dropped := metrics.DefaultRegistry().OutboxDropped.Load()

	require.NoError(t, manager.Start(context.Background()))
	assert.Equal(t, "renamed", provider.receive(t))
	require.NoError(t, manager.Stop(context.Background()))

	assert.Equal(t, dropped+1, metrics.DefaultRegistry().OutboxDropped.Load())
	assert.Zero(t, store.len())
}

func TestJobsHeldBeforeStartArePersisted(t *testing.T) {
	provider := newScriptedProvider(nil)
	store := newMemOutbox()
	manager := outboxManager(t, provider, nil)
	manager.Notify("before attach")
	require.NoError(t, manager.AttachOutbox(store))
	manager.Notify("after attach")
	dead := metrics.DefaultRegistry().DeliveryDeadLetters.Load()

	require.NoError(t, manager.Stop(context.Background()))

	assert.Equal(t, 2, store.len(), "a crash before Start loses nothing")
	assert.Equal(t, dead, metrics.DefaultRegistry().DeliveryDeadLetters.Load(),
		"jobs the next session sends are not dead letters")
}

func TestOutboxSweepDropsExpiredRecords(t *testing.T) {
	clk := newFakeClock()
	box := newOutbox(newMemOutbox(), clk.Now)
	old := box.add(deliverJob{kind: jobMessage, msg: "old",
		queued: clk.Now()}, "slack")
	clk.Advance(outboxMaxAge)
	fresh := box.add(deliverJob{kind: jobMessage, msg: "fresh"}, "slack")
	clk.Advance(time.Minute)
	dropped := metrics.DefaultRegistry().OutboxDropped.Load()

	box.sweepExpired(clk.Now())

	box.mu.Lock()
	_, oldLive := box.live[old]
	_, freshLive := box.live[fresh]
	box.mu.Unlock()
	assert.False(t, oldLive)
	assert.True(t, freshLive)
	assert.Equal(t, dropped+1, metrics.DefaultRegistry().OutboxDropped.Load())
}
