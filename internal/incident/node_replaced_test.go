package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func goneNodeIncident(t *testing.T, reason string) (*rig, *Incident) {
	t.Helper()
	r := newRig(t, Config{})
	id := inventory.CoreID(kube.KindNode, "", "n1")
	p := &Incident{Root: id, Opened: at(0),
		Members: map[detection.Key]detection.Finding{}}
	f := sig(id, reason, detection.Critical)
	p.Members[f.Key()] = f
	return r, p
}

func TestNodeReplacedNeedsRealFailure(t *testing.T) {
	r, p := goneNodeIncident(t, reasons.NodeNotReady)
	assert.True(t, nodeReplaced(r.model, p), "failed and gone")

	r, p = goneNodeIncident(t, reasons.NodeDraining)
	assert.False(t, nodeReplaced(r.model, p), "a drain is not a failure")

	// Findings cleared before the resolve: remembered root reasons count.
	r, p = goneNodeIncident(t, reasons.NodeDraining)
	p.Members = nil
	p.rootReasons = map[string]struct{}{reasons.NodeHeartbeatStale: {}}
	assert.True(t, nodeReplaced(r.model, p))
}

func TestSameNodeGroup(t *testing.T) {
	r := newRig(t, Config{})
	node := func(name string) inventory.EntityID {
		return inventory.CoreID(kube.KindNode, "", name)
	}
	poolA := inventory.CoreID(kube.KindNodePool, "", "a")
	poolB := inventory.CoreID(kube.KindNodePool, "", "b")
	r.relate(node("old"), inventory.PartOf, poolA)
	r.relate(node("same"), inventory.PartOf, poolA)
	r.relate(node("other"), inventory.PartOf, poolB)

	assert.True(t, sameNodeGroup(r.model, node("old"), node("same")))
	assert.False(t, sameNodeGroup(r.model, node("old"), node("other")))
	// A root without pool or zone gives nothing to compare: match.
	assert.True(t, sameNodeGroup(r.model, node("bare"), node("other")))
}
