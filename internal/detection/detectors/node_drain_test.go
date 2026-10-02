package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cordonedNode is cordoned at cordon and reports Ready and MemoryPressure
// with the given statuses since notReady.
func cordonedNode(
	cordon, notReady time.Time, ready, memory string,
) (*inventory.Model, inventory.Entity) {
	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindNode, Name: "node-1"}
	model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: cordon, Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrUnschedulable: inventory.Bool(true),
		},
	})
	setCondition(model, id, "Ready", ready, "KubeletNotReady", "down",
		notReady)
	setCondition(model, id, "MemoryPressure", memory, "", "", notReady)
	entity, _ := model.Entity(id)
	return model, entity
}

func reasonsOf(findings []detection.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, s := range findings {
		out = append(out, s.Reason)
	}
	return out
}

func TestNodeCordonedKeepsPressureFindings(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model, node := cordonedNode(now.Add(-10*time.Minute),
		now.Add(-5*time.Minute), "True", "True")

	got := reasonsOf(NewNode(0).Detect(
		testDetectorContext(model, now), node))

	assert.ElementsMatch(t, []string{
		reasons.NodeDraining, reasons.MemoryPressure,
	}, got)
}

func TestNodeCordonedNotReadyInsideDrainEnvelopeIsExpected(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model, node := cordonedNode(now.Add(-10*time.Minute),
		now.Add(-5*time.Minute), "False", "False")

	got := reasonsOf(NewNode(0).Detect(
		testDetectorContext(model, now), node))

	assert.Equal(t, []string{reasons.NodeDraining}, got)
}

func TestNodeCordonedNotReadyPastDrainEnvelopeIsReported(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model, node := cordonedNode(now.Add(-drainEnvelope-time.Minute),
		now.Add(-drainEnvelope), "False", "False")

	got := reasonsOf(NewNode(0).Detect(
		testDetectorContext(model, now), node))

	assert.ElementsMatch(t, []string{
		reasons.NodeDraining, reasons.NodeNotReady,
	}, got)
}

func TestNodeNotReadyBeforeCordonIsReported(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model, node := cordonedNode(now.Add(-2*time.Minute),
		now.Add(-10*time.Minute), "Unknown", "False")

	got := reasonsOf(NewNode(0).Detect(
		testDetectorContext(model, now), node))

	assert.ElementsMatch(t, []string{
		reasons.NodeDraining, reasons.NodeNotReady,
	}, got)
}
