package detectors

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

var daemonNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// daemonPod is a Failed pod of DaemonSet agent on node n1, whose
// attributes the caller sets.
func daemonPod(nodeAttrs map[string]inventory.Value,
) (*inventory.Model, inventory.Entity) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	attrs := conditionAttrs(map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(daemonNow.Add(-24 * time.Hour)),
		kube.AttrReady:   inventory.Bool(true),
	}, "Ready", "True", "KubeletReady", daemonNow.Add(-24*time.Hour))
	for name, value := range nodeAttrs {
		attrs[name] = value
	}
	observeEntity(m, node, daemonNow.Add(-24*time.Hour), attrs)
	ds := newID(kube.KindDaemonSet, "kube-system", "agent")
	observeEntity(m, ds, daemonNow.Add(-24*time.Hour), nil)
	pod := newID(kube.KindPod, "kube-system", "agent-x")
	observeEntity(m, pod, daemonNow, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	})
	relateEntity(m, pod, inventory.OwnedBy, ds)
	relateEntity(m, pod, inventory.RunsOn, node)
	return m, entityOf(m, pod)
}

func detectDaemonPod(m *inventory.Model, pod inventory.Entity,
) []detection.Finding {
	return NewPod(PodThresholds{}).Detect(
		testDetectorContext(m, daemonNow), pod)
}

// The node's lifecycle ends its DaemonSet pods: an autoscaler that
// consolidates a node must not make each of them a failure.
func TestPodDaemonSetFailureOnALeavingNodeIsNotReported(t *testing.T) {
	cases := map[string]map[string]inventory.Value{
		"deleting": {kube.AttrDeleting: inventory.Bool(true)},
		"cordoned": {kube.AttrUnschedulable: inventory.Bool(true)},
		"scale-down": {kube.AttrTaints: inventory.Text(
			"ToBeDeletedByClusterAutoscaler:NoSchedule")},
		"out of service": {kube.AttrTaints: inventory.Text(
			"node.kubernetes.io/out-of-service:NoExecute")},
	}
	for name, attrs := range cases {
		m, pod := daemonPod(attrs)

		assert.Empty(t, detectDaemonPod(m, pod), name)
	}
}

func TestPodDaemonSetFailureOnAGoneNodeIsNotReported(t *testing.T) {
	m := newTestModel()
	ds := newID(kube.KindDaemonSet, "kube-system", "agent")
	observeEntity(m, ds, daemonNow, nil)
	id := newID(kube.KindPod, "kube-system", "agent-x")
	observeEntity(m, id, daemonNow, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	})
	relateEntity(m, id, inventory.OwnedBy, ds)
	relateEntity(m, id, inventory.RunsOn, newID(kube.KindNode, "", "gone"))

	assert.Empty(t, detectDaemonPod(m, entityOf(m, id)))
}

func TestPodDaemonSetFailureOnASettledNodeIsReported(t *testing.T) {
	m, pod := daemonPod(nil)

	got := detectDaemonPod(m, pod)

	require.Len(t, got, 1)
	assert.Equal(t, reasons.PodFailed, got[0].Reason)
}

func TestPodFailureOfAnotherOwnerOnALeavingNodeIsReported(t *testing.T) {
	m, _ := daemonPod(map[string]inventory.Value{
		kube.AttrDeleting: inventory.Bool(true)})
	rs := newID(kube.KindReplicaSet, "kube-system", "web-rs")
	observeEntity(m, rs, daemonNow, nil)
	other := newID(kube.KindPod, "kube-system", "web-x")
	observeEntity(m, other, daemonNow, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	})
	relateEntity(m, other, inventory.OwnedBy, rs)
	relateEntity(m, other, inventory.RunsOn, newID(kube.KindNode, "", "n1"))

	got := detectDaemonPod(m, entityOf(m, other))

	require.Len(t, got, 1)
}
