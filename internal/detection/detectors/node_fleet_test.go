package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// silentFleet lists two nodes whose kubelets both went silent at t0.
func silentFleet(m *inventory.Model) inventory.EntityID {
	for _, name := range []string{"n1", "n2"} {
		put(m, newID(kube.KindNode, "", name), t0, conditionAttrs(nil,
			"Ready", "Unknown", nodeStatusUnknown, t0))
	}
	return newID(kube.KindNode, "", "n1")
}

// fleetPod lists a pod in phase with the Ready condition status.
func fleetPod(
	m *inventory.Model, name, phase, ready string,
) inventory.EntityID {
	id := newID(kube.KindPod, "ns", name)
	put(m, id, t0, conditionAttrs(map[string]inventory.Value{
		kube.AttrPhase: inventory.Text(phase),
	}, "Ready", ready, "", t0))
	return id
}

// heartbeatAt reports whether n1's stale heartbeat is a finding a
// minute after the fleet went silent, past the usual wait.
func heartbeatAt(m *inventory.Model, node inventory.EntityID) bool {
	got := evaluate(NewNode(0), m, t0.Add(time.Minute), node, nil)
	for _, f := range got.Findings {
		if f.Reason == reasons.NodeHeartbeatStale {
			return true
		}
	}
	return false
}

// Finished pods (a completed Job run, an evicted pod) are not ready
// for good; they do not end the hold of a fleet gone silent.
func TestSilentFleetIgnoresFinishedPods(t *testing.T) {
	m := newTestModel()
	node := silentFleet(m)
	fleetPod(m, "backup-29", "Succeeded", "False")
	fleetPod(m, "evicted", "Failed", "False")

	assert.False(t, heartbeatAt(m, node), "the whole fleet is held")
}

// A running pod that is not ready is felt: the hold ends and the node
// is reported at the usual pace.
func TestSilentFleetWithUnreadyPodIsReported(t *testing.T) {
	m := newTestModel()
	node := silentFleet(m)
	fleetPod(m, "web-1", "Running", "False")

	assert.True(t, heartbeatAt(m, node))
}
