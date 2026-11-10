package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// One node that goes after a wave is over is ordinary churn: it
// neither starts the wave again nor moves the end of its hold.
func TestScalingWaveIsNotRestartedByOneLateNode(t *testing.T) {
	r := waveRig(t, 8)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	end := at(time.Minute).Add(scaleQuiet + scaleGrace)

	r.removeNodes(at(7*time.Minute), nodeName(3))

	assert.Equal(t, end, r.m.Scaling(at(7*time.Minute)).Expected)
	assert.False(t, r.m.Scaling(end).Held, "the hold ends as it was")
	r.removeNodes(at(10*time.Minute), nodeName(4))
	assert.False(t, r.m.Scaling(end).Held,
		"two late nodes are no wave either")
}

// Three fresh changes after a wave is over are a new wave, counted
// from the first of them.
func TestScalingWaveStartsAgainFromFreshChanges(t *testing.T) {
	r := waveRig(t, 8)
	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))

	r.removeNodes(at(7*time.Minute), nodeName(3))
	r.removeNodes(at(8*time.Minute), nodeName(4), nodeName(5))

	state := r.m.Scaling(at(8 * time.Minute))
	assert.True(t, state.Held)
	assert.Equal(t, at(7*time.Minute), state.Since)
}

// applySynced applies the model at now with the Node kind synced or
// not.
func (r *rig) applySynced(now time.Time, synced bool) {
	s := r.snapshot(now)
	s.Synced = func(inventory.Kind) bool { return synced }
	r.m.Apply(s, []inventory.EntityID{nodeName(0)}, nil)
}

// At a start the nodes arrive one by one. Until all are listed the
// model is no baseline: the rest of the list is not nodes joining.
func TestScalingWaveTakesNoBaselineFromHalfANodeList(t *testing.T) {
	r := newRig(t, Config{})
	ready := map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)}
	r.setAttrs(nodeName(0), ready)
	r.applySynced(at(0), false)
	for i := 1; i < 5; i++ {
		r.setAttrs(nodeName(i), ready)
	}
	r.applySynced(at(time.Second), true)

	assert.False(t, r.m.Scaling(at(time.Second)).Held,
		"the first full list is the baseline")

	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))
	assert.True(t, r.m.Scaling(at(time.Minute)).Held,
		"a real wave after it is seen")
}

// The baseline is taken as soon as the nodes are listed, even when no
// node changes at that moment: the first change after it counts.
func TestScalingWaveBaselineNeedsNoNodeChange(t *testing.T) {
	r := newRig(t, Config{})
	for i := range 5 {
		r.setAttrs(nodeName(i), map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(true)})
	}
	r.applySynced(at(0), false)
	r.m.Apply(r.snapshot(at(time.Second)), nil, nil)

	r.removeNodes(at(time.Minute), nodeName(0), nodeName(1), nodeName(2))

	assert.True(t, r.m.Scaling(at(time.Minute)).Held)
}

// A cluster that shrank for good is judged by the size it has now, not
// by the most nodes it ever had.
func TestScalingWaveUsesTheCurrentSize(t *testing.T) {
	r := waveRig(t, 40)
	var gone []inventory.EntityID
	for i := range 30 {
		gone = append(gone, nodeName(i))
	}
	r.removeNodes(at(time.Minute), gone...)

	r.removeNodes(at(time.Hour), nodeName(30), nodeName(31), nodeName(32))

	assert.True(t, r.m.Scaling(at(time.Hour)).Held,
		"three of ten nodes are a wave")
}
