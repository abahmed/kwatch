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

// A finding about a node that no detector clears (one read from the
// events the node left behind) ends with the node: once the Node is
// deleted, its incident resolves and says the node was removed.
func TestNodeIncidentResolvesWhenTheNodeIsDeleted(t *testing.T) {
	r := waveRig(t, 2)
	node := nodeName(0)
	pressure := sig(node, reasons.NodePSIHigh, detection.Warning)
	r.raise(at(time.Minute), pressure)
	r.tick(at(5 * time.Minute))
	r.removeNodes(at(10*time.Minute), node)
	// The finding is still active: nothing cleared it.
	r.m.Apply(r.snapshot(at(11*time.Minute)), []inventory.EntityID{node},
		nil)

	var resolved *Incident
	for now := at(11 * time.Minute); now.Before(at(3*time.Hour)) &&
		resolved == nil; now = now.Add(time.Minute) {
		r.tick(now)
		if p := r.of(node); p.State == Resolved {
			resolved = &p
		}
	}

	require.NotNil(t, resolved, "node incident never resolved")
	assert.Contains(t, resolved.Removed, node)
}

// A node that is only missing from the model (never listed) is not
// claimed removed: its finding stays.
func TestNodeNeverSeenIsNotRemoved(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n9")
	r.raise(at(0), sig(node, reasons.NodePSIHigh, detection.Warning))

	assert.Len(t, r.only().Members, 1)
	assert.False(t, r.m.nodeRemoved(node))
}
