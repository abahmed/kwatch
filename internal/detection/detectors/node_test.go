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

func TestNodeReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Hour)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
		},
	})

	// Set Ready condition to True
	setCondition(model, id, "Ready", "True", "KubeletReady", "", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	assert.Empty(t, findings)
}

func TestNodeNotReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-2 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	// Set Ready condition to False
	setCondition(model, id, "Ready", "False", "KubeletNotReady",
		"Node not ready", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.NodeNotReady, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "NotReady")
}

func TestNodeUnknown(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-2 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	// Set Ready condition to Unknown
	setCondition(model, id, "Ready", "Unknown", "Unknown",
		"Node stopped reporting", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.NodeNotReady, findings[0].Reason)
	assert.Contains(t, findings[0].Summary, "stopped reporting")
}

func TestNodeCordoned(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	cordoned := now.Add(-10 * time.Minute)

	node := buildNode("node-1", cordoned, map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true),
	})

	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, node)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.NodeDraining, findings[0].Reason)
	assert.Equal(t, detection.Info, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "cordoned")
}

func TestNodeDraining(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	deleting := now.Add(-10 * time.Minute)

	node := buildNode("node-1", deleting, map[string]inventory.Value{
		kube.AttrDeleting: inventory.Bool(true),
	})

	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, node)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.NodeDraining, findings[0].Reason)
	assert.Contains(t, findings[0].Summary, "being removed")
}

func TestNodeStuckTerminating(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	deleting := now.Add(-31 * time.Minute)

	node := buildNode("node-1", deleting, map[string]inventory.Value{
		kube.AttrDeleting: inventory.Bool(true),
	})

	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, node)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.NodeStuckTerminating, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
}

func TestNodeMemoryPressure(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	setCondition(model, id, "MemoryPressure", "True", "MemoryPressure",
		"", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.MemoryPressure, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
}

func TestNodeDiskPressure(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	setCondition(model, id, "DiskPressure", "True", "DiskPressure", "",
		since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.DiskPressure, findings[0].Reason)
}

func TestNodeNetworkUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	setCondition(model, id, "NetworkUnavailable", "True",
		"NetworkNotReady", "", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.NetworkUnavailable, findings[0].Reason)
}

// A removal taint announces a drain before the node is cordoned or
// deleted.
func TestNodeRemovalTaintIsADrain(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for taints, want := range map[string]string{
		"ToBeDeletedByClusterAutoscaler=1700000000:NoSchedule": "scale-down",
		"node.kubernetes.io/out-of-service=nodeshutdown:NoExecute": "marked " +
			"out of service",
		"gpu=true:NoSchedule": "",
	} {
		node := buildNode("n1", now.Add(-time.Minute),
			map[string]inventory.Value{
				kube.AttrTaints:   inventory.Text(taints),
				"condition.Ready": inventory.Text("True"),
			})
		ok, drain := drainFinding(node)
		if want == "" {
			assert.False(t, ok, taints)
			continue
		}
		require.True(t, ok, taints)
		assert.Equal(t, reasons.NodeDraining, drain.Reason)
		assert.Contains(t, drain.Summary, want)
	}
}
