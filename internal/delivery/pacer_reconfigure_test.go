package delivery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSendPacerRetainKeepsStateOfConfiguredProviders(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var p sendPacer
	require.True(t, p.admit("slack", 1, now))
	require.False(t, p.admit("slack", 1, now))
	require.True(t, p.admit("teams", 1, now))
	p.summaries = map[string]*overflowSummary{
		"slack": {total: 3, since: now},
		"teams": {total: 1, since: now},
	}
	p.block("slack", now.Add(time.Minute))

	p.retain(map[string]struct{}{"slack": {}})

	require.False(t, p.admit("slack", 1, now),
		"an exhausted budget must stay exhausted after reconfiguration")
	require.Equal(t, 3, p.summaries["slack"].total)
	require.NotContains(t, p.summaries, "teams")
	require.Contains(t, p.blocked, "slack")
	require.NotContains(t, p.hourly, "teams")
}
