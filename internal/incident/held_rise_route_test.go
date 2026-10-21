package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// An announcement that a startup summary or an outage hold collected is
// held: nobody was told yet, but the incident may rise to a page while it
// waits. That update goes out through the hold, and the route recorded
// for the incident must still learn the page's severity and reasons, or
// the resolve is routed without the pager route that the page matched.
func TestHeldIncidentThatRisesToPageWidensItsAnnouncedRoute(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	web := sig(pod, reasons.CrashLoopBackOff, detection.Warning)
	r.raise(at(0), web)
	ds := r.tick(at(DefaultSettle))
	wantAction(t, ds, Announce, ReasonSettled)
	id := ds[0].Incident.ID
	r.m.HoldAnnouncement(id)

	node := nodeID()
	r.raise(at(3*time.Minute), nodeDown())
	r.cause(pod, node, "node n1 is down")
	r.apply(at(3*time.Minute), detection.Changed, web)
	rise := r.tick(at(3*time.Minute + DefaultReviseSettle + time.Second))

	require.NotEmpty(t, rise)
	got := r.of(node)
	require.NotNil(t, got.AnnouncedRoute)
	assert.Equal(t, "critical", got.AnnouncedRoute.Severity)
	assert.Contains(t, got.AnnouncedRoute.Reasons, reasons.NodeNotReady)
}
