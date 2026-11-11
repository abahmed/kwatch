package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// The week's resolve numbers have an alerted-only variant: issues that
// settled on their own leave the alerted count and median.
func TestWeekCountsAlertedResolvesApart(t *testing.T) {
	quiet := untoldPage("q", ago(2*time.Hour))
	told := opened("t", "shop", incident.Page, ago(3*time.Hour))
	told.State, told.Resolved = incident.Resolved, ago(time.Hour)
	items := []incident.Incident{quiet, told}

	got := buildTrends(items, trendNow, time.UTC, 0)
	assert.Equal(t, 2, got.Resolve.Count)
	assert.Equal(t, 1, got.ResolveAlerted.Count)
	assert.Equal(t, int64(2*3600), got.ResolveAlerted.MedianSeconds)

	days := insightDays(items, trendNow, time.UTC)
	today := days[len(days)-1]
	assert.Equal(t, 2, today.Resolved)
	assert.Equal(t, 1, today.ResolvedAlerted)
}
