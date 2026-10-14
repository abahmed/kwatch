package incident

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Nodes that are already leaving when kwatch starts are not fresh
// displacements: their start time is unknown, so a failure that comes
// back on them continues the resolved incident.
func TestNodesLeavingAtStartupAreNotDisplacements(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	node := entity(kube.KindNode, "n1")
	r.setAttrs(node, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	r.setAttrs(web.Entity, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(at(-48 * time.Hour))})
	r.relate(web.Entity, inventory.RunsOn, node)
	announced(t, r, web)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	r.tick(at(2*time.Minute + DefaultHold))

	again := at(5 * time.Minute)
	r.raise(again, web)
	assert.True(t, r.m.churn.at.IsZero(), "no displacement was seen")
	wantAction(t, r.tick(again.Add(DefaultReviseSettle)), Update,
		ReasonFailingAgain)
}

// A node that a restored node incident stood on, and that is gone when
// the model first has nodes, was removed while kwatch was down: that is
// a displacement at that moment.
func TestNodeRemovedWhileDownIsADisplacement(t *testing.T) {
	src := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	down := sig(node, "NodeNotReady", detection.Critical)
	src.raise(at(0), down)
	src.tick(at(DefaultSettle))

	dst := newRig(t, Config{})
	dst.setAttrs(entity(kube.KindNode, "n2"), map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	dst.m.Restore(src.m.Export(), at(time.Minute))
	dst.apply(at(2*time.Minute), detection.Raised)
	assert.Equal(t, at(2*time.Minute), dst.m.churn.at)
}

// A restored node incident whose node is still there is no displacement.
func TestRestoredNodeStillPresentIsNoDisplacement(t *testing.T) {
	src := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	src.raise(at(0), sig(node, "NodeNotReady", detection.Critical))
	src.tick(at(DefaultSettle))

	dst := newRig(t, Config{})
	dst.setAttrs(node, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	dst.m.Restore(src.m.Export(), at(time.Minute))
	dst.apply(at(2*time.Minute), detection.Raised)
	assert.True(t, dst.m.churn.at.IsZero())
}

// The memory of moved roots does not outlive RepageWindow, even when no
// incident moves again.
func TestFormerRootsArePrunedByTick(t *testing.T) {
	r := newRig(t, Config{})
	for i := range 5 {
		p := &Incident{ID: fmt.Sprintf("i%d", i), rerootedAt: at(0)}
		r.m.incidents[p.ID] = p
		r.m.former[fmt.Sprintf("root%d", i)] = p.ID
	}
	r.tick(at(RepageWindow + time.Minute))
	assert.Empty(t, r.m.former)
}

// A tick with no node change does not scan the nodes again, and a node
// that changed between ticks is still seen by the next one.
func TestQuietTickDoesNotScanNodes(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	r.setAttrs(node, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	r.apply(at(0), detection.Raised) // the baseline look
	scans := r.m.churn.scans

	r.tick(at(time.Minute))
	r.tick(at(2 * time.Minute))
	assert.Equal(t, scans, r.m.churn.scans, "quiet ticks do not scan")

	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: at(time.Minute),
		Entity: node, Attributes: map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(false)}})
	require.NoError(t, err)
	r.markNode(node)
	r.tick(at(3 * time.Minute))
	assert.Equal(t, scans+1, r.m.churn.scans)
	assert.Equal(t, at(3*time.Minute), r.m.churn.at, "the change was seen")
}

// An Apply that names a node looks at the nodes; one that does not
// leaves them alone.
func TestApplyScansNodesOnlyWhenOneChanged(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	r.setAttrs(node, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	r.apply(at(0), detection.Raised)
	scans := r.m.churn.scans

	r.apply(at(time.Minute), detection.Raised)
	assert.Equal(t, scans, r.m.churn.scans)
	r.m.Apply(r.snapshot(at(2*time.Minute)),
		[]inventory.EntityID{node}, nil)
	assert.Equal(t, scans+1, r.m.churn.scans)
}
