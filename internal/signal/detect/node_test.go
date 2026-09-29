package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestNodeReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Hour)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrPhase: knowledge.Text("Running"),
		},
	})

	// Set Ready condition to True
	setCondition(model, id, "Ready", "True", "KubeletReady", "", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	assert.Empty(t, signals)
}

func TestNodeNotReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-2 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	// Set Ready condition to False
	setCondition(model, id, "Ready", "False", "KubeletNotReady",
		"Node not ready", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonNodeNotReady, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "NotReady")
}

func TestNodeUnknown(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-2 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	// Set Ready condition to Unknown
	setCondition(model, id, "Ready", "Unknown", "Unknown",
		"Node stopped reporting", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonNodeNotReady, signals[0].Reason)
	assert.Contains(t, signals[0].Summary, "stopped reporting")
}

func TestNodeCordoned(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	cordoned := now.Add(-10 * time.Minute)

	node := buildNode("node-1", cordoned, map[string]knowledge.Value{
		kube.AttrUnschedulable: knowledge.Bool(true),
	})

	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, node)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonNodeDraining, signals[0].Reason)
	assert.Equal(t, signal.Info, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "cordoned")
}

func TestNodeDraining(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	deleting := now.Add(-10 * time.Minute)

	node := buildNode("node-1", deleting, map[string]knowledge.Value{
		kube.AttrDeleting: knowledge.Bool(true),
	})

	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, node)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonNodeDraining, signals[0].Reason)
	assert.Contains(t, signals[0].Summary, "being removed")
}

func TestNodeStuckTerminating(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	deleting := now.Add(-31 * time.Minute)

	node := buildNode("node-1", deleting, map[string]knowledge.Value{
		kube.AttrDeleting: knowledge.Bool(true),
	})

	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, node)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonNodeStuckTerminating, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
}

func TestNodeMemoryPressure(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	setCondition(model, id, "MemoryPressure", "True", "MemoryPressure",
		"", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonMemoryPressure, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
}

func TestNodeDiskPressure(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	setCondition(model, id, "DiskPressure", "True", "DiskPressure", "",
		since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonDiskPressure, signals[0].Reason)
}

func TestNodeNetworkUnavailable(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	setCondition(model, id, "NetworkUnavailable", "True",
		"NetworkNotReady", "", since)

	entity, _ := model.Entity(id)
	detector := NewNode(90 * time.Second)
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonNetworkUnavailable, signals[0].Reason)
}
