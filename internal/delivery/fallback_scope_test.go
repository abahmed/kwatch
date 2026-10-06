package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var fastRetry = retryConfig{maxAttempts: 1, delay: time.Millisecond}

func fallbackPair(
	primary, fallback Provider,
) (*Manager, *providerEntry) {
	m := managerWithEntries([]providerEntry{
		{provider: primary, retry: fastRetry, fallbackName: "Fallback"},
		{provider: fallback, retry: fastRetry},
	})
	return m, &managerEntries(m)[0]
}

// A chat fallback that accepts a pager's resolve has not closed the
// alert, so the pager keeps being retried.
func TestChatFallbackDoesNotSwallowPagerResolve(t *testing.T) {
	pager := &skippingProvider{errorRecorderProvider{
		name: "Pager", err: errors.New("timeout")}}
	chat := &errorRecorderProvider{name: "Fallback"}
	m, entry := fallbackPair(pager, chat)

	got := m.deliver(context.Background(), entry, resolveJob("k"))

	assert.Equal(t, outcomeRetryLater, got)
	assert.Zero(t, chat.callCount,
		"chat reads the resolve as its own message, not the pager's copy")
}

// Other messages still use the fallback: an update has no alert state.
func TestChatFallbackStillTakesPagerUpdates(t *testing.T) {
	pager := &skippingProvider{errorRecorderProvider{
		name: "Pager", err: errors.New("timeout")}}
	chat := &errorRecorderProvider{name: "Fallback"}
	m, entry := fallbackPair(pager, chat)
	job := incidentJob("k", "shop")
	job.incident.Revision = 3

	got := m.deliver(context.Background(), entry, job)

	assert.Equal(t, outcomeDelivered, got)
	assert.Equal(t, 1, chat.callCount)
}

// A pager fallback skips plain messages and summaries, so accepting one
// delivers nothing: the primary keeps being retried.
func TestPagerFallbackDoesNotSwallowInformationalMessages(t *testing.T) {
	chat := &errorRecorderProvider{name: "Primary",
		err: errors.New("timeout")}
	pager := &skippingProvider{errorRecorderProvider{name: "Fallback"}}
	m, entry := fallbackPair(chat, pager)

	summary := incidentJob("startup/1", "shop")
	for name, job := range map[string]deliverJob{
		"notice":  {kind: jobMessage, msg: "kwatch started"},
		"summary": summary,
	} {
		err := m.deliverFallback(context.Background(),
			&managerEntries(m)[1], "Primary", job)
		assert.ErrorIs(t, err, errFallbackNotRouted, name)
		assert.Equal(t, outcomeRetryLater,
			m.deliver(context.Background(), entry, job), name)
	}
	assert.Zero(t, pager.callCount)
}
