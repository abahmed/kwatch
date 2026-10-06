package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

// An announced incident whose pod then begins to crash-loop says so
// once, and says it as worse, not as a recovery.
func TestCrashLoopAfterAnnounceIsOneUpdate(t *testing.T) {
	result := replayNamed(t, "crashloop-after-announce")

	require.Len(t, result.Messages, 2)
	assert.Equal(t, incident.Announce, result.Decisions[0].Action)
	assert.Equal(t, incident.Update, result.Decisions[1].Action)
	assert.Equal(t, incident.ReasonMaterialChange,
		result.Decisions[1].Reason)
	assert.Contains(t, result.Messages[1].Note, "is getting worse")
	assert.Contains(t, result.Messages[1].Note, "now crash-loops")
}

// A notify-tier incident that fails again within two hours is the same
// incident: one announcement, then "failing again" in the same thread.
// The third failure comes inside the longer hold of an incident that
// already re-opened, so it adds nothing.
func TestNotifyFlapReopensTheSameIncident(t *testing.T) {
	result := replayNamed(t, "notify-flap-reopens")

	require.Len(t, decisionsOf(result, incident.Announce, ""), 1)
	again := decisionsOf(result, incident.Update, incident.ReasonFailingAgain)
	require.Len(t, again, 1)
	assert.Equal(t, incident.Notify, again[0].Incident.Tier)
	assert.Contains(t, result.Messages[2].Note,
		"is failing again: 2nd time in two hours")
	for i, d := range result.Decisions {
		assert.Equal(t, result.Decisions[0].Incident.ID, d.Incident.ID,
			"decision %d", i)
	}
	assert.Len(t, result.Incidents, 1)
	assert.Len(t, result.Messages, 3)
}

// lastIncidentDecisions lists the decisions of the incident that opened
// last: the third day of a known crash loop.
func lastIncidentDecisions(result replay.Result) []incident.Decision {
	last := result.Incidents[len(result.Incidents)-1].ID
	var out []incident.Decision
	for _, d := range result.Decisions {
		if d.Incident.ID == last {
			out = append(out, d)
		}
	}
	return out
}

// A crash loop people heard about on two earlier days is known: when it
// lasts only six minutes, boot noise, nobody is interrupted.
func TestKnownBootStillDemotes(t *testing.T) {
	result := replayNamed(t, "known-boot-still-demotes")

	for _, d := range lastIncidentDecisions(result) {
		assert.Equal(t, incident.Digest, d.Incident.Tier,
			"decision %s of the third day", d.Reason)
	}
}

// The same crash loop that lasts past the boot window with nothing
// ready is not routine: it is announced like a new failure.
func TestKnownCrashLoopPastBootIsNotDemoted(t *testing.T) {
	result := replayNamed(t, "known-crashloop-past-boot")

	day := lastIncidentDecisions(result)
	require.NotEmpty(t, day)
	first := day[0]
	// The digest held its announcement; promoted out of it, the update
	// is the first message and is written as an announcement.
	require.Equal(t, incident.Announce, first.Action)
	assert.Equal(t, incident.ReasonMaterialChange, first.Reason)
	assert.Equal(t, incident.Notify, first.Incident.Tier)
	opened := first.Incident.Opened
	var at time.Time
	for i, d := range result.Decisions {
		if d.Incident.ID == first.Incident.ID {
			at = result.Times[i]
			break
		}
	}
	assert.Equal(t, kube.BootWindow, at.Sub(opened),
		"promoted when the boot window ends")
}

// An autoscaler incident does not resolve while the workload it scales
// is down and its pods crash-loop.
func TestHPAIncidentDoesNotResolveWhileWorkloadIsBroken(t *testing.T) {
	result := replayNamed(t, "hpa-resolve-while-workload-broken")

	for _, d := range result.Decisions {
		assert.NotEqual(t, incident.Resolve, d.Action,
			"%s resolved while the workload was down", d.Incident.Root)
	}
	var hpa bool
	for _, p := range result.Incidents {
		if p.Root.Kind == kube.KindHPA {
			hpa = true
			assert.NotEqual(t, incident.Resolved, p.State)
		}
	}
	assert.True(t, hpa, "the autoscaler had an incident")
}

// Pods crashing and recovering five times behind a Service is one
// incident. The reasoning's flips between the pod, the Service and the
// Deployment are not material changes: after the announcement the only
// thing said is that the incident is flapping.
func TestCauseFlipDoesNotRepostAMaterialChange(t *testing.T) {
	result := replayNamed(t, "cause-flip-stable")

	assert.Empty(t, decisionsOf(result, incident.Update,
		incident.ReasonMaterialChange))
	assert.Empty(t, decisionsOf(result, incident.Update,
		incident.ReasonCauseRevised))
	assert.LessOrEqual(t, len(result.Messages), 2)
	assert.Len(t, result.Incidents, 1)
}
