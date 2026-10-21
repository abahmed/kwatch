package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/replay"
)

// A restored incident whose notify-tier findings are back right after the
// sources synced is named by the restart summary one settle later, not
// when the restore grace ends ten minutes on: waiting for the rest of the
// restored incidents left a loud problem unmentioned all that time.
func TestRestartSummaryNamesLoudProblemsOneSettleAfterTheSync(t *testing.T) {
	store := newPersistingStore()
	_, failing := restartFirstSession()
	c := failing.cluster
	c.after(45 * time.Minute)
	first := c.log()
	firstRun := restartReplay(t, first, store, replay.Options{
		SyncAt: first.Start, Tail: 5 * time.Minute})

	restartAt := firstRun.End.Add(restartGap)
	again := newCluster(scenarioStart, "")
	again.now = restartAt
	again.list(remaining(c)...)
	again.after(30 * time.Minute)
	second := again.log()
	second.Start = restartAt.UTC()
	secondRun := restartReplay(t, second, store, replay.Options{
		SyncAt: restartAt, Tail: 20 * time.Minute})

	var at time.Time
	for i, m := range secondRun.Messages {
		if strings.Contains(m.Title, "restarted") {
			at = secondRun.Times[i]
		}
	}
	require.False(t, at.IsZero(), "no restart summary: %s",
		describeDecisions(secondRun))
	wait := at.Sub(restartAt)
	assert.LessOrEqual(t, wait, incident.DefaultSettle+time.Minute,
		"it waited %s for the grace", wait)
}
