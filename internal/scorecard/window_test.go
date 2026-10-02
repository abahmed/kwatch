package scorecard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPeakInWindowFindsTheBusiestSpan(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(minutes ...int) []time.Time {
		var out []time.Time
		for _, m := range minutes {
			out = append(out, start.Add(time.Duration(m)*time.Minute))
		}
		return out
	}
	assert.Equal(t, 0, PeakInWindow(nil, time.Hour))
	assert.Equal(t, 3, PeakInWindow(at(0, 10, 59, 70), time.Hour))
	assert.Equal(t, 2, PeakInWindow(at(0, 60, 61), time.Hour),
		"a timestamp exactly one window later starts the next window")
	assert.Equal(t, 3, PeakInWindow(at(90, 0, 91, 91), 2*time.Minute),
		"input order does not matter")
}

func TestScoreReportsPeakHourAndMostMessages(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	report := Score(scoreEntries(start))
	assert.Equal(t, 3, report.PeakPerHour)
	assert.Equal(t, 3, report.MaxPerIncident)
	assert.InDelta(t, 50, report.RecreatedPercent(), 0.001)
}
