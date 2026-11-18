package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The timeline holds the objects of open incidents with the times of
// their decisions, and lets a resolved incident's objects go.
func TestTimelineHoldsOpenIncidentsWithTheirDecisions(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	pod := inventory.CoreID(kube.KindPod, "shop", "api-1")
	f := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff"}
	open := incident.Incident{ID: "a", Root: root, State: incident.Open,
		Members: map[detection.Key]detection.Finding{f.Key(): f}}
	gone := incident.Incident{ID: "b", State: incident.Resolved,
		Root: inventory.CoreID(kube.KindDeployment, "shop", "old")}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var holds timelineHolds

	got := holds.decided(at, []incident.Decision{
		{Action: incident.Announce, Incident: open},
		{Action: incident.Resolve, Incident: gone}},
		[]incident.Incident{open, gone})

	require.Len(t, got, 2)
	assert.Equal(t, []time.Time{at}, got[root.String()].Marks)
	assert.Contains(t, got, pod.String())
	assert.NotContains(t, got, gone.Root.String())

	later := holds.decided(at.Add(time.Hour), []incident.Decision{
		{Action: incident.Update, Incident: open}},
		[]incident.Incident{open})
	assert.Equal(t, []time.Time{at, at.Add(time.Hour)},
		later[root.String()].Marks, "the opening stays marked")
}
