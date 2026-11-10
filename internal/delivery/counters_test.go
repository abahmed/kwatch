package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/metrics"
)

func recentFailures(m *Manager) int {
	now := m.nowTime()
	return m.failures.since(now.Add(-time.Minute), now)
}

// A send cut off by shutdown is not a failure of the provider.
func TestShutdownCancelIsNotCountedAsFailedSend(t *testing.T) {
	provider := &errorRecorderProvider{name: "P", err: context.Canceled}
	m, entry := ctxlessEntry(provider)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = m.dispatch(ctx, entry, incidentJob("k", "ns"),
		deliverOpts{retry: fastRetry})

	assert.Zero(t, recentFailures(m))

	provider.err = errors.New("boom")
	_ = m.dispatch(context.Background(), entry, incidentJob("k", "ns"),
		deliverOpts{retry: fastRetry})
	assert.Equal(t, 1, recentFailures(m), "a real failure still counts")
}

func ctxlessEntry(p Provider) (*Manager, *providerEntry) {
	m := managerWithEntries([]providerEntry{{provider: p, retry: fastRetry}})
	return m, &managerEntries(m)[0]
}

// A job shutdown could not send and nothing will resend is dropped, and
// counted as such.
func TestShutdownDeadLetterWithoutOutboxCountsAsDropped(t *testing.T) {
	m, entry := ctxlessEntry(&errorRecorderProvider{name: "P"})
	dropped := &metrics.DefaultRegistry().NotificationsDropped
	before := dropped.Load()

	m.deferJob(entry, incidentJob("k", "ns"), errDeliveryShutdown)

	assert.Equal(t, before+1, dropped.Load())
}

// A job the overflow summary stands in for is counted as dropped and
// saturated, not also as a dead letter.
func TestSummarizedJobIsNotAlsoADeadLetter(t *testing.T) {
	m, entry := ctxlessEntry(&errorRecorderProvider{name: "P"})
	registry := metrics.DefaultRegistry()
	dead := registry.DeliveryDeadLetters.Load()
	dropped := registry.NotificationsDropped.Load()

	m.recordQueueDrop(*entry, incidentJob("k", "ns"))

	assert.Equal(t, dead, registry.DeliveryDeadLetters.Load())
	assert.Equal(t, dropped+1, registry.NotificationsDropped.Load())
}
