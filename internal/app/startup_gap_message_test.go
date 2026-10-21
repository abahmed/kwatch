package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func gapStartup(gapReport bool) *startupManager {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return &startupManager{
		shouldNotify: true, currentVersion: "v1.2.0",
		downtime: time.Hour, gapEnd: now, gapReport: gapReport,
		hideProHints: true, now: func() time.Time { return now },
	}
}

func TestStartupManagerCommunityGapKeepsTodaysMessage(t *testing.T) {
	s := gapStartup(false)

	msg, ok := s.StartupMessage()
	_, gapOK := s.GapMessage("report")

	assert.True(t, ok)
	assert.Equal(t, "🟠 kwatch v1.2.0 started; nothing was monitored "+
		"from 11:00 to 12:00 UTC (1h), so anything that broke then went "+
		"unreported.", msg)
	assert.False(t, gapOK, "Community sends no gap report")
}

func TestStartupManagerProGapWaitsForTheReport(t *testing.T) {
	s := gapStartup(true)

	_, ok := s.StartupMessage()
	msg, gapOK := s.GapMessage("kwatch was not running from 11:00 to " +
		"12:00 UTC (1h). Compared with before, nothing changed and " +
		"nothing crashed.")
	_, again := s.GapMessage("report")

	assert.False(t, ok, "the startup message goes out with the report")
	assert.True(t, gapOK)
	assert.Equal(t, "🟠 kwatch v1.2.0 started.\n\nkwatch was not running "+
		"from 11:00 to 12:00 UTC (1h). Compared with before, nothing "+
		"changed and nothing crashed.", msg)
	assert.False(t, again, "a restarted pipeline does not send it twice")
	assert.Equal(t, time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC),
		s.gap().gap.Start)
}
