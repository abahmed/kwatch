package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// A failure that returns after its incident resolved finds that incident
// even when a cause revision moved its root while it recovered: within
// two hours it reopens as "failing again", under the same ID and back at
// the root the reasoning names now.
func TestMovedRootReturnReopensItsIncident(t *testing.T) {
	result := replayNamed(t, "moved-root-returns")

	require.Len(t, result.Incidents, 1, "no second incident")
	p := result.Incidents[0]
	assert.Equal(t, "configmap/shop/settings", p.Root.String())
	again := decisionsOf(result, incident.Update, incident.ReasonFailingAgain)
	require.Len(t, again, 1)
	assert.Equal(t, p.ID, again[0].Incident.ID)
	assert.Equal(t, 2, p.RepeatCount)
}
