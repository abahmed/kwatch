package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ebsNodes is a DaemonSet with desired pods, ready of them ready, first
// seen at seen (kwatch's first sight of it).
func ebsNodes(
	m *inventory.Model, seen time.Time, desired, ready float64,
) inventory.EntityID {
	id := newID(kube.KindDaemonSet, "kube-system", "ebs-csi-node")
	put(m, id, seen, map[string]inventory.Value{
		kube.AttrDesiredReplicas: inventory.Number(desired),
		kube.AttrReadyReplicas:   inventory.Number(ready),
		kube.AttrUnavailable:     inventory.Number(desired - ready),
	})
	return id
}

// kwatch starts, replays an event from before its start, and finds the
// DaemonSet fully ready: the transient is over, so nothing is reported.
func TestEventBeforeKwatchSawTheObjectHealthyIsHistory(t *testing.T) {
	m := newTestModel()
	id := ebsNodes(m, t0, 5, 5)
	warn(m, id, t0.Add(-6*time.Minute), "FailedDaemonPod",
		"Found failed daemon pod kube-system/ebs-csi-node-x on node n1")

	eval := evaluate(Event{}, m, t0.Add(time.Minute), id, nil)

	assert.Empty(t, eval.Findings)
}

// An event that arrives after the object was seen healthy is news.
func TestEventAfterTheObjectWasSeenHealthyStillCounts(t *testing.T) {
	m := newTestModel()
	id := ebsNodes(m, t0, 5, 5)
	warn(m, id, t0.Add(2*time.Minute), "FailedDaemonPod",
		"Found failed daemon pod")

	eval := evaluate(Event{}, m, t0.Add(3*time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "FailedDaemonPod", eval.Findings[0].Reason)
}

// A DaemonSet that is still short of ready pods keeps its finding.
func TestEventForAnUnhealthyObjectStillCounts(t *testing.T) {
	m := newTestModel()
	id := ebsNodes(m, t0, 5, 4)
	warn(m, id, t0.Add(-6*time.Minute), "FailedDaemonPod",
		"Found failed daemon pod")

	eval := evaluate(Event{}, m, t0.Add(time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
}

func TestEventBeforeADeploymentWasAvailableIsHistory(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindDeployment, "shop", "api")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(2),
		kube.AttrReadyReplicas: inventory.Number(2),
		kube.AttrAvailable:     inventory.Number(2),
	})
	warn(m, id, t0.Add(-5*time.Minute), "FailedCreate",
		"Error creating: pods \"api-x\" is forbidden")

	eval := evaluate(Event{}, m, t0.Add(time.Minute), id, nil)

	assert.Empty(t, eval.Findings)
}
