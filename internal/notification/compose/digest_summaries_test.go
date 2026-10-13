package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// The digest says how many lines it rendered and quotes its grouped,
// unchanged and wake-up lines, so the audit log can prove them.
func TestDigestListingCountsLinesAndQuotesSummaries(t *testing.T) {
	var opened []incident.Decision
	for i := range 5 {
		opened = append(opened, leftoverDecision(i, 1))
	}
	from := time.Date(2026, 10, 6, 6, 51, 0, 0, time.UTC)
	extras := DigestExtras{
		Wake: &WakeLine{Started: 42, From: from,
			To: from.Add(12 * time.Minute), Blips: 5},
		Unchanged: []Ongoing{leftoverOngoing(1), leftoverOngoing(2)},
	}

	msg := Writer{}.DigestWith(opened, nil, nil, extras, extrasNow)

	assert.Equal(t, len(msg.Lines), msg.Listed.Lines)
	joined := strings.Join(msg.Listed.Summaries, "\n")
	assert.Contains(t, joined, "Cluster waking up")
	assert.Contains(t, joined, "5 expired TLS Secrets")
	assert.Contains(t, joined, "unchanged")
	assert.LessOrEqual(t, len(msg.Listed.Summaries), maxSummaryLines)
}
