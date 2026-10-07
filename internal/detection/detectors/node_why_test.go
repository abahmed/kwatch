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

// whyNode is a node whose Ready condition is not True since two
// minutes before podNodeNow.
func whyNode(
	t *testing.T, status, reason, message string,
) (*inventory.Model, inventory.EntityID) {
	t.Helper()
	m := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "ip-10-0-1-2"}
	observeEntity(m, id, podNodeNow, map[string]inventory.Value{})
	setCondition(m, id, "Ready", status, reason, message,
		podNodeNow.Add(-2*time.Minute))
	return m, id
}

func whyFinding(
	t *testing.T, m *inventory.Model, id inventory.EntityID,
) detection.Finding {
	t.Helper()
	entity, ok := m.Entity(id)
	require.True(t, ok)
	found := NewNode(90*time.Second).Detect(
		testDetectorContext(m, podNodeNow), entity)
	for _, f := range found {
		if f.Reason == "NodeNotReady" {
			return f
		}
	}
	require.Fail(t, "no NodeNotReady finding")
	return detection.Finding{}
}

func whyValues(f detection.Finding, label string) []string {
	var out []string
	for _, e := range f.Evidence {
		if e.Label == label {
			out = append(out, e.Value)
		}
	}
	return out
}

func TestNodeNotReadyNamesTheReadyStatus(t *testing.T) {
	m, id := whyNode(t, "Unknown", "NodeStatusUnknown",
		"Kubelet stopped posting node status.")
	got := whyFinding(t, m, id)
	assert.Equal(t, []string{"Unknown"},
		whyValues(got, detection.EvidenceReadyStatus))
	assert.Equal(t, []string{"Kubelet stopped posting node status."},
		whyValues(got, "message"))
}

func TestNodeNotReadyListsOtherTrueConditions(t *testing.T) {
	m, id := whyNode(t, "False", "KubeletNotReady",
		"container runtime is down")
	entity, _ := m.Entity(id)
	attrs := map[string]inventory.Value{}
	for k, v := range entity.Attributes {
		attrs[k] = v.Value
	}
	key := kube.ConditionKey("DiskPressure")
	attrs[key] = inventory.Text("True")
	attrs[key+kube.AttrConditionMessage] = inventory.Text(
		"kubelet has disk pressure")
	attrs[kube.ConditionKey("MemoryPressure")] = inventory.Text("False")
	observeEntity(m, id, podNodeNow, attrs)
	got := whyFinding(t, m, id)
	assert.Equal(t, []string{`DiskPressure "kubelet has disk pressure"`},
		whyValues(got, detection.EvidenceNodeCondition))
}

func TestNodeNotReadyQuotesRecentNodeWarnings(t *testing.T) {
	m, id := whyNode(t, "False", "KubeletNotReady", "PLEG is not healthy")
	note := func(at time.Duration, reason, message string, warn bool) {
		m.Apply(inventory.Observation{Kind: inventory.Noted,
			Source: "test", At: podNodeNow, Entity: id,
			Note: inventory.Note{At: podNodeNow.Add(-at),
				Source: "kubelet", Reason: reason, Message: message,
				Count: 1, Warning: warn}})
	}
	note(time.Minute, "ImageGCFailed", "failed to get image stats", true)
	note(2*time.Minute, "Rebooted", "Node has been rebooted", true)
	note(3*time.Minute, "SystemOOM", "System OOM encountered", true)
	note(time.Minute, "NodeNotReady", "Node status is now: NodeNotReady",
		true)
	note(time.Minute, "Starting", "Starting kubelet.", false)
	note(time.Hour, "ContainerGCFailed", "too old", true)
	got := whyFinding(t, m, id)
	assert.Equal(t, []string{
		`ImageGCFailed "failed to get image stats"`,
		`Rebooted "Node has been rebooted"`},
		whyValues(got, detection.EvidenceNodeEvent),
		"newest first, at most two, no echo of the NotReady itself")
}
