package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestRenderOverflowSummaryIsAPlainSentence(t *testing.T) {
	cases := []struct {
		name  string
		state overflowSummary
		want  string
	}{
		{"one", overflowSummary{
			byReason: map[string]int{"web crashed": 1}, total: 1,
		}, "1 notification was not sent one by one to avoid flooding " +
			"this channel: web crashed ×1."},
		{"several", overflowSummary{
			byReason: map[string]int{"web crashed": 2, "db slow": 1},
			total:    3,
		}, "3 notifications were not sent one by one to avoid flooding " +
			"this channel: web crashed ×2, db slow ×1."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderOverflowSummary(&tc.state)
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, ":warning:")
			assert.NotContains(t, got, "/incidents")
		})
	}
}

// skippingProvider drops plain messages, like a paging provider.
type skippingProvider struct{ errorRecorderProvider }

func (*skippingProvider) SkipsPlainMessages() bool { return true }

func TestFlushOverflowSummaryCountsSkipOnPagingProvider(t *testing.T) {
	provider := &skippingProvider{errorRecorderProvider{name: "Pager"}}
	am := newTestManager()
	entry := &providerEntry{provider: provider}
	am.addToOverflowSummary(provider.Name(), incidentJob("k", "default"))
	before := metrics.DefaultRegistry().DeliveryDigestSkipped.Load()

	am.flushOverflowSummary(context.Background(), entry)

	assert.Zero(t, provider.callCount, "the summary is not sent")
	assert.Equal(t, int64(1),
		metrics.DefaultRegistry().DeliveryDigestSkipped.Load()-before)
	_, text := am.takeOverflowSummary(provider.Name())
	assert.Empty(t, text, "a skipped summary is not retried")
}

// A summary the provider rejects for good is given up, not re-queued to
// fail and be counted again at every flush.
func TestFlushOverflowSummaryDropsPermanentlyRejectedSummary(t *testing.T) {
	provider := &errorRecorderProvider{name: "Chat",
		err: transport.Permanent(errors.New("invalid token"))}
	am := managerWithEntries([]providerEntry{{
		provider: provider,
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
	}})
	entry := &managerEntries(am)[0]
	am.addToOverflowSummary("Chat", incidentJob("k", "default"))
	registry := metrics.DefaultRegistry()
	before := registry.DeliveryDeadLetters.Load()

	am.flushOverflowSummary(context.Background(), entry)
	am.flushOverflowSummary(context.Background(), entry)

	assert.Equal(t, 1, provider.callCount, "not tried again")
	assert.Equal(t, int64(1), registry.DeliveryDeadLetters.Load()-before)
	_, text := am.takeOverflowSummary("Chat")
	assert.Empty(t, text)
}

// One that fails in a way that passes stays pending for the next flush.
func TestFlushOverflowSummaryKeepsSummaryOnTemporaryFailure(t *testing.T) {
	provider := &errorRecorderProvider{name: "Chat",
		err: errors.New("connection refused")}
	am := managerWithEntries([]providerEntry{{
		provider: provider,
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
	}})
	entry := &managerEntries(am)[0]
	am.addToOverflowSummary("Chat", incidentJob("k", "default"))

	am.flushOverflowSummary(context.Background(), entry)

	_, text := am.takeOverflowSummary("Chat")
	assert.NotEmpty(t, text)
}
