package delivery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

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
