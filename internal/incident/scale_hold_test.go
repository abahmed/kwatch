package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeName names the test nodes n0, n1, ...
func nodeName(i int) inventory.EntityID {
	return entity(kube.KindNode, "n"+string(rune('0'+i)))
}

// waveRig is a rig with count nodes, looked at once (the baseline).
func waveRig(t *testing.T, count int) *rig {
	t.Helper()
	r := newRig(t, Config{})
	for i := range count {
		r.setAttrs(nodeName(i), map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(true)})
	}
	r.m.Apply(r.snapshot(at(0)), nil, nil)
	return r
}

// removeNodes deletes the nodes from the model at now and lets the
// manager look.
func (r *rig) removeNodes(now time.Time, ids ...inventory.EntityID) {
	r.t.Helper()
	for _, id := range ids {
		_, err := r.model.Apply(inventory.Observation{
			Kind: inventory.Gone, Source: "test", At: now, Entity: id})
		require.NoError(r.t, err)
	}
	r.m.Apply(r.snapshot(now), ids, nil)
}

// downNode is a critical NotReady finding of node: it pages (node-lost).
func downNode(node inventory.EntityID) detection.Finding {
	return sig(node, reasons.NodeNotReady, detection.Critical)
}

// firstDecision ticks from from to until in steps and returns the first
// decision and when it came.
func firstDecision(
	r *rig, from, until time.Time,
) (Decision, time.Time, bool) {
	for now := from; !now.After(until); now = now.Add(5 * time.Second) {
		if ds := r.tick(now); len(ds) > 0 {
			return ds[0], now, true
		}
	}
	return Decision{}, time.Time{}, false
}

// A cordoned node that goes NotReady while three of five nodes are
// removed is the scale-down at work: its page waits for the wave to end
// plus the grace, then pages when it still fails.
func TestScalingWaveHoldsAPageOfAScalingSideEffect(t *testing.T) {
	r := waveRig(t, 5)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	r.setAttrs(nodeName(3), map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	r.raise(at(2*time.Minute), downNode(nodeName(3)))

	d, when, ok := firstDecision(r, at(2*time.Minute), at(time.Hour))

	require.True(t, ok)
	assert.Equal(t, Announce, d.Action)
	assert.Equal(t, Page, d.Incident.Tier)
	end := at(time.Minute).Add(scaleQuiet + scaleGrace)
	assert.False(t, when.Before(end), "paged at %s, before %s", when, end)
	assert.True(t, when.Before(end.Add(10*time.Second)))
}

// While held, the incident says why, and the cluster's scaling state
// says since when and until when.
func TestScalingWaveSaysWhyThePageWaits(t *testing.T) {
	r := waveRig(t, 5)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	r.setAttrs(nodeName(3), map[string]inventory.Value{
		kube.AttrDeleting: inventory.Bool(true)})
	r.raise(at(2*time.Minute), downNode(nodeName(3)))
	wantNone(t, r.tick(at(3*time.Minute)))

	why := WhyOf(r.of(nodeName(3)), at(3*time.Minute))
	assert.Equal(t, ScalingHeldText, why.PagingHeld)
	assert.Contains(t, why.StillOpen, ScalingHeldText)
	state := r.m.Scaling(at(3 * time.Minute))
	assert.True(t, state.Held)
	assert.Equal(t, at(time.Minute), state.Since)
	assert.Equal(t, at(time.Minute).Add(scaleQuiet+scaleGrace),
		state.Expected)
	assert.False(t, r.m.Scaling(at(time.Hour)).Held)
}

// A held page whose failure clears during the wave closes without a
// message.
func TestScalingWaveHeldPageThatClearsSendsNothing(t *testing.T) {
	r := waveRig(t, 5)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	r.setAttrs(nodeName(3), map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	lost := downNode(nodeName(3))
	r.raise(at(2*time.Minute), lost)
	wantNone(t, r.tick(at(3*time.Minute)))
	r.clear(at(4*time.Minute), lost)

	_, _, ok := firstDecision(r, at(4*time.Minute), at(time.Hour))

	assert.False(t, ok)
}

// A node that fails without being taken out is a lost node, not the
// scaling: it pages at once, wave or not.
func TestScalingWaveNeverHoldsANodeLostOnItsOwn(t *testing.T) {
	r := waveRig(t, 5)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	r.raise(at(2*time.Minute), downNode(nodeName(4)))

	d, when, ok := firstDecision(r, at(2*time.Minute), at(time.Hour))

	require.True(t, ok)
	assert.Equal(t, Page, d.Incident.Tier)
	assert.True(t, when.Before(at(3*time.Minute)), "paged at %s", when)
}

// A crash loop joining a held page ends the hold.
func TestScalingWaveCrashLoopIsNeverHeld(t *testing.T) {
	r := waveRig(t, 5)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	node := nodeName(3)
	r.setAttrs(node, map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	crash := sig(node, reasons.CrashLoopBackOff, detection.Critical)
	r.raise(at(2*time.Minute), downNode(node), crash)

	_, when, ok := firstDecision(r, at(2*time.Minute), at(time.Hour))

	require.True(t, ok)
	assert.True(t, when.Before(at(3*time.Minute)), "paged at %s", when)
}

// Two nodes removed is ordinary churn, not a wave: the page goes out
// after the page settle.
func TestScalingWaveNeedsEnoughNodes(t *testing.T) {
	r := waveRig(t, 5)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1))
	r.setAttrs(nodeName(3), map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	r.raise(at(2*time.Minute), downNode(nodeName(3)))

	_, when, ok := firstDecision(r, at(2*time.Minute), at(time.Hour))

	require.True(t, ok)
	assert.True(t, when.Before(at(3*time.Minute)), "paged at %s", when)
	assert.False(t, r.m.Scaling(at(2*time.Minute)).Held)
}

// Nodes added count as much as nodes removed; a large cluster needs a
// fifth of its nodes to change.
func TestScalingWaveThreshold(t *testing.T) {
	w := scaleWave{size: 5}
	assert.Equal(t, scaleMinNodes, w.threshold())
	w.size = 40
	assert.Equal(t, 8, w.threshold())
	w.size = 41
	assert.Equal(t, 9, w.threshold())

	r := waveRig(t, 3)
	for i := 3; i < 6; i++ {
		r.setAttrs(nodeName(i), map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(true)})
	}
	r.m.Apply(r.snapshot(at(time.Minute)), nil, nil)
	assert.True(t, r.m.Scaling(at(time.Minute)).Held)
}

// endlessWave ticks from from while one more node, from n<next> on,
// goes every four minutes, so the wave never ends, and returns when
// the first decision came.
func endlessWave(r *rig, from time.Time, next int) time.Time {
	for now := from; now.Before(at(time.Hour)); now = now.Add(
		5 * time.Second) {
		if now.Sub(t0)%(4*time.Minute) == time.Minute && next < 12 {
			r.removeNodes(now, nodeName(next))
			next++
		}
		if len(r.tick(now)) > 0 {
			return now
		}
	}
	return time.Time{}
}

// A cluster that never stops scaling holds a page at most
// scaleMaxHold from the failure's onset. An onset from before the
// wave (the fixture's is t0) counts from the wave's start.
func TestScalingWaveHoldIsCapped(t *testing.T) {
	r := waveRig(t, 13)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	held := nodeName(12)
	r.setAttrs(held, map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	r.raise(at(2*time.Minute), downNode(held))

	paged := endlessWave(r, at(2*time.Minute), 3)

	assert.Equal(t, at(time.Minute).Add(scaleMaxHold), paged)
}

// A hold is forgotten once its incident stopped settling.
func TestScalingWaveForgetsHoldsOfAnnouncedIncidents(t *testing.T) {
	r := waveRig(t, 6)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	r.setAttrs(nodeName(3), map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	r.raise(at(2*time.Minute), downNode(nodeName(3)))
	_, _, ok := firstDecision(r, at(2*time.Minute), at(time.Hour))
	require.True(t, ok)
	require.Len(t, r.m.scaleHolds, 1)

	r.removeNodes(at(time.Hour), nodeName(4))

	assert.Empty(t, r.m.scaleHolds)
}
