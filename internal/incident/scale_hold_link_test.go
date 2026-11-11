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

// wavePod puts pod name into the model, created at created and running
// on node; a zero node leaves it unscheduled.
func (r *rig) wavePod(
	name string, created time.Time, node inventory.EntityID,
) inventory.EntityID {
	r.t.Helper()
	pod := entity(kube.KindPod, name)
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created)})
	if !node.IsZero() {
		r.relate(pod, inventory.RunsOn, node)
	}
	return pod
}

// notReadyAt is a critical not-ready finding of pod that began at
// since.
func notReadyAt(pod inventory.EntityID, since time.Time) detection.Finding {
	s := sig(pod, reasons.NotReady, detection.Critical)
	s.Since = since
	return s
}

// scaleDown is five nodes of which three are removed at one minute.
func scaleDown(t *testing.T) *rig {
	t.Helper()
	r := waveRig(t, 5)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	return r
}

// waveEnd is when the hold of scaleDown's wave ends.
func waveEnd() time.Time {
	return at(time.Minute).Add(scaleQuiet + scaleGrace)
}

// firstAt is when the first decision after from came.
func firstAt(t *testing.T, r *rig, from time.Time) time.Time {
	t.Helper()
	_, when, ok := firstDecision(r, from, at(time.Hour))
	require.True(t, ok)
	return when
}

// A pod the scale-down moved (created during it) that is not ready yet
// is the scaling at work: it is held until the wave is over.
func TestScalingWaveHoldsAPodItMoved(t *testing.T) {
	r := scaleDown(t)
	pod := r.wavePod("web", at(90*time.Second), nodeName(3))
	r.raise(at(2*time.Minute), notReadyAt(pod, at(2*time.Minute)))

	when := firstAt(t, r, at(2*time.Minute))

	assert.False(t, when.Before(waveEnd()), "told at %s", when)
}

// The same failure of a pod that has run for an hour on a node the
// wave never touched has nothing to do with the scaling: it is told
// after the usual settle.
func TestScalingWaveNeverHoldsAnOutageOnUntouchedNodes(t *testing.T) {
	r := scaleDown(t)
	pod := r.wavePod("pay", at(-time.Hour), nodeName(3))
	r.raise(at(2*time.Minute), notReadyAt(pod, at(2*time.Minute)))

	when := firstAt(t, r, at(2*time.Minute))

	assert.True(t, when.Before(at(4*time.Minute)), "told at %s", when)
}

// A pod on a node the wave is removing, and one that waits for a node,
// are tied to the wave whatever their age.
func TestScalingWaveLinksPodsOnItsNodesAndUnscheduledOnes(t *testing.T) {
	r := scaleDown(t)
	r.setAttrs(nodeName(3), map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	leaving := r.wavePod("web", at(-time.Hour), nodeName(3))
	waiting := r.wavePod("api", at(-time.Hour), inventory.EntityID{})
	r.raise(at(2*time.Minute), notReadyAt(leaving, at(2*time.Minute)),
		notReadyAt(waiting, at(2*time.Minute)))

	wantNone(t, tickEvery(r, 2*time.Minute, 10*time.Minute))
}

// A pull that failed is an error, not a wait: it is never held, even
// for a pod the wave moved.
func TestScalingWaveNeverHoldsAFailedImagePull(t *testing.T) {
	r := scaleDown(t)
	pod := r.wavePod("web", at(90*time.Second), nodeName(3))
	pull := sig(pod, reasons.ImagePullBackOff, detection.Critical)
	pull.Since = at(2 * time.Minute)
	r.raise(at(2*time.Minute), pull)

	when := firstAt(t, r, at(2*time.Minute))

	assert.True(t, when.Before(at(4*time.Minute)), "told at %s", when)
}

// A failure that began well before the wave is not its side effect.
func TestScalingWaveNeverHoldsAFailureOlderThanTheWave(t *testing.T) {
	r := scaleDown(t)
	pod := r.wavePod("web", at(90*time.Second), nodeName(3))
	old := at(time.Minute).Add(-scaleOnsetSlack - time.Second)
	r.raise(at(2*time.Minute), notReadyAt(pod, old))

	when := firstAt(t, r, at(2*time.Minute))

	assert.True(t, when.Before(at(4*time.Minute)), "told at %s", when)
}

// A node that joined during the wave may be not ready while it boots,
// but only for scaleJoinGrace: after that it is a broken node, and its
// page does not wait for the rest of the wave.
func TestScalingWaveHoldsAJoinedNodeOnlyWhileItBoots(t *testing.T) {
	r := waveRig(t, 3)
	joined := at(time.Minute)
	for i := 3; i < 6; i++ {
		_, err := r.model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", At: joined,
			Entity: nodeName(i)})
		require.NoError(t, err)
	}
	r.m.Apply(r.snapshot(joined), []inventory.EntityID{nodeName(3)}, nil)
	r.raise(at(2*time.Minute), downNode(nodeName(3)))

	when := firstAt(t, r, at(2*time.Minute))

	end := joined.Add(scaleJoinGrace)
	assert.False(t, when.Before(end), "paged at %s", when)
	assert.True(t, when.Before(end.Add(10*time.Second)),
		"paged at %s, the wave's hold ends at %s", when, waveEnd())
}

// While users lose traffic or nothing serves, the hold is capped at
// scaleMaxHoldServing from the onset: a node that was taken away and
// never replaced looks like scaling for as long as the wave lasts.
func TestScalingWaveHoldIsShorterWhenNothingServes(t *testing.T) {
	r := waveRig(t, 13)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	pod := r.wavePod("web", at(90*time.Second), nodeName(12))
	onset := at(2 * time.Minute)
	r.raise(onset, notReadyAt(pod, onset))
	r.m.mu.Lock()
	r.m.lookup(pod).servingDown = true
	r.m.mu.Unlock()

	told := endlessWave(r, onset, 3)

	assert.Equal(t, onset.Add(scaleMaxHoldServing), told)
}

// The cap counts from the failure's onset, not from the incident's
// opening: a failure that another hold kept back for a while (node
// churn) does not start a fresh half hour when it opens.
func TestScalingWaveCapCountsFromTheOnset(t *testing.T) {
	r := waveRig(t, 13)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	pod := r.wavePod("web", at(90*time.Second), nodeName(12))
	onset := at(2 * time.Minute)
	r.removeNodes(at(5*time.Minute), nodeName(3))
	r.raise(at(6*time.Minute), notReadyAt(pod, onset))

	told := endlessWave(r, at(6*time.Minute), 4)

	assert.Equal(t, onset.Add(scaleMaxHold), told)
}
