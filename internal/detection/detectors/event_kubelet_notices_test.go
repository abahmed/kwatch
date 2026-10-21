package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The kubelet writes these Warning notices on every healthy node start
// or pod start: they are worth the digest, never an interruption.
func TestEventBenignKubeletNoticesAreInformational(t *testing.T) {
	for _, reason := range []string{"InvalidDiskCapacity", "Rebooted",
		"DNSConfigForming", "CheckLimitsForResolvConf"} {
		kind := kube.KindNode
		if reason == "DNSConfigForming" ||
			reason == "CheckLimitsForResolvConf" {
			kind = kube.KindPod
		}
		m := newTestModel()
		id := newID(kind, "", "x")
		observeEntity(m, id, t0, nil)
		noteEntity(m, id, reason, "kubelet notice", 1, t0)

		got := Event{}.Detect(testDetectorContext(m, t0.Add(time.Minute)),
			entityOf(m, id))

		require.Len(t, got, 1, reason)
		assert.Equal(t, detection.Info, got[0].Severity, reason)
	}
}

func readyNode(m *inventory.Model, status string, since time.Time,
) inventory.EntityID {
	id := newID(kube.KindNode, "", "n1")
	observeEntity(m, id, since, conditionAttrs(nil, "Ready", status,
		"KubeletReady", since))
	return id
}

// A kubelet note older than the node's last turn to Ready was about a
// node that has since recovered or finished starting.
func TestEventKubeletNoteBeforeTheNodeTurnedReadyIsHistory(t *testing.T) {
	m := newTestModel()
	id := readyNode(m, "True", t0.Add(2*time.Minute))
	noteEntity(m, id, "FailedNodeAllocatableEnforcement",
		"Failed to update Node Allocatable Limits", 1, t0)

	got := Event{}.Detect(testDetectorContext(m, t0.Add(5*time.Minute)),
		entityOf(m, id))

	assert.Empty(t, got)
}

func TestEventKubeletNoteAfterTheNodeTurnedReadyIsReported(t *testing.T) {
	m := newTestModel()
	id := readyNode(m, "True", t0)
	noteEntity(m, id, "FailedNodeAllocatableEnforcement",
		"Failed to update Node Allocatable Limits", 1,
		t0.Add(2*time.Minute))

	got := Event{}.Detect(testDetectorContext(m, t0.Add(5*time.Minute)),
		entityOf(m, id))

	require.Len(t, got, 1)
}

// A node that is not Ready now keeps every note: they may be why.
func TestEventKubeletNoteOfANotReadyNodeIsKept(t *testing.T) {
	m := newTestModel()
	id := readyNode(m, "False", t0.Add(2*time.Minute))
	noteEntity(m, id, "FailedNodeAllocatableEnforcement",
		"Failed to update Node Allocatable Limits", 1, t0)

	got := Event{}.Detect(testDetectorContext(m, t0.Add(5*time.Minute)),
		entityOf(m, id))

	require.Len(t, got, 1)
}
