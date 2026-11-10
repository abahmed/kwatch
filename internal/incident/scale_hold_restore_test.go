package incident

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// restartedInWave holds the page of a cordoned node during a
// scale-down, saves the incidents at three minutes as the store does
// (through JSON) and restores them into a new manager on the same
// cluster. The new manager has seen no wave.
func restartedInWave(t *testing.T) *rig {
	t.Helper()
	old := scaleDown(t)
	old.setAttrs(nodeName(3), map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	old.raise(at(2*time.Minute), downNode(nodeName(3)))
	wantNone(t, old.tick(at(3*time.Minute)))
	raw, err := json.Marshal(old.m.Export())
	require.NoError(t, err)
	var recs []Record
	require.NoError(t, json.Unmarshal(raw, &recs))
	require.Len(t, recs, 1)
	require.Equal(t, waveEnd(), recs[0].ScaleHeldUntil.UTC(),
		"the end of the hold is saved with the incident")

	fresh := newRig(t, Config{})
	fresh.model = old.model
	fresh.m.Restore(recs, at(3*time.Minute))
	return fresh
}

// A restart during a scaling wave keeps the hold: the page that waited
// for the nodes to settle still waits, and goes out when the hold it
// was given ends.
func TestScalingHoldSurvivesARestart(t *testing.T) {
	r := restartedInWave(t)
	back := at(3*time.Minute + 5*time.Second)
	r.raise(back, downNode(nodeName(3)))
	require.False(t, r.m.Scaling(back).Held, "no wave after the restart")

	d, when, ok := firstDecision(r, back, at(time.Hour))

	require.True(t, ok)
	assert.Equal(t, Page, d.Incident.Tier)
	assert.False(t, when.Before(waveEnd()), "paged at %s", when)
	assert.True(t, when.Before(waveEnd().Add(10*time.Second)))
}

// The saved hold counts only while every failure is still a scaling
// side effect: a crash loop back after the restart is told at once.
func TestRestoredScalingHoldEndsWithAnotherFailure(t *testing.T) {
	r := restartedInWave(t)
	back := at(3*time.Minute + 5*time.Second)
	r.raise(back, downNode(nodeName(3)),
		sig(nodeName(3), reasons.CrashLoopBackOff, detection.Critical))

	_, when, ok := firstDecision(r, back, at(time.Hour))

	require.True(t, ok)
	assert.True(t, when.Before(at(5*time.Minute)), "told at %s", when)
}
