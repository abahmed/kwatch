package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func gatedPod(attrs map[string]inventory.Value) (*inventory.Model,
	inventory.EntityID,
) {
	m := newTestModel()
	id := newID(kube.KindPod, "shop", "api-1")
	base := map[string]inventory.Value{
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(t0),
	}
	for k, v := range attrs {
		base[k] = v
	}
	put(m, id, t0, base)
	return m, id
}

func TestPodNotReadyNamesUnmetReadinessGate(t *testing.T) {
	m, id := gatedPod(map[string]inventory.Value{
		kube.AttrReadinessGates:        inventory.Text("lb-health,mesh"),
		kube.ConditionKey("lb-health"): inventory.Text("False"),
		kube.ConditionKey("lb-health") + kube.AttrConditionReason: inventory.
			Text("Draining"),
		kube.ConditionKey("mesh"): inventory.Text("True"),
	})

	got := evaluate(NewPod(PodThresholds{}), m,
		t0.Add(10*time.Minute), id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ContainersNotReady, got[0].Reason)
	require.Len(t, got[0].Evidence, 1)
	assert.Equal(t, EvidenceReadinessGate, got[0].Evidence[0].Label)
	assert.Equal(t, "lb-health=False (Draining)", got[0].Evidence[0].Value)
}

func TestPodNotReadyTreatsUnsetGateAsMissing(t *testing.T) {
	m, id := gatedPod(map[string]inventory.Value{
		kube.AttrReadinessGates: inventory.Text("mesh"),
	})

	got := evaluate(NewPod(PodThresholds{}), m,
		t0.Add(10*time.Minute), id, nil).Findings

	require.Len(t, got, 1)
	require.Len(t, got[0].Evidence, 1)
	assert.Equal(t, "mesh=Missing", got[0].Evidence[0].Value)
}

func TestPodNotReadyWithoutGatesHasNoGateEvidence(t *testing.T) {
	m, id := gatedPod(nil)

	got := evaluate(NewPod(PodThresholds{}), m,
		t0.Add(10*time.Minute), id, nil).Findings

	require.Len(t, got, 1)
	for _, ev := range got[0].Evidence {
		assert.NotEqual(t, EvidenceReadinessGate, ev.Label)
	}
}
