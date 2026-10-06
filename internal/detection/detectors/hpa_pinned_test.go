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

// limitedHPA is an HPA whose current and desired replicas equal max and
// whose ScalingLimited condition is true, with the given floor.
func limitedHPA(
	floor, maximum float64,
) (*inventory.Model, inventory.EntityID) {
	model := newTestModel()
	id := newID(kube.KindHPA, "default", "app")
	put(model, id, t0, map[string]inventory.Value{
		kube.AttrCurrentReplicas: inventory.Number(maximum),
		kube.AttrDesiredReplicas: inventory.Number(maximum),
		kube.AttrMaxReplicas:     inventory.Number(maximum),
		kube.AttrMinReplicas:     inventory.Number(floor),
	})
	setCondition(model, id, "ScalingLimited", "True", "TooManyReplicas",
		"at the maximum", t0)
	return model, id
}

func TestHPAMaxedOutSkipsPinnedAutoscaler(t *testing.T) {
	model, id := limitedHPA(2, 2)
	got := evaluate(HPA{}, model, t0.Add(time.Minute), id, nil)
	assert.Empty(t, got.Findings)
}

func TestHPAMaxedOutKeepsRealSaturation(t *testing.T) {
	model, id := limitedHPA(1, 2)
	got := evaluate(HPA{}, model, t0.Add(time.Minute), id, nil)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, reasons.HPAMaxedOut, got.Findings[0].Reason)
}

// TestHPAMetricsFailureWaitsForMetricsGrace: a blip of the metrics API
// that ends within HPAMetricsGrace never becomes a finding.
func TestHPAMetricsFailureWaitsForMetricsGrace(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindHPA, "default", "app")
	put(m, id, t0, nil)
	setCondition(m, id, "ScalingActive", "False",
		"FailedGetResourceMetric", "metrics API unavailable", t0)

	early := evaluate(HPA{}, m, t0.Add(DefaultConditionGrace), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, HPAMetricsGrace-DefaultConditionGrace,
		early.RecheckAfter)

	late := evaluate(HPA{}, m, t0.Add(HPAMetricsGrace), id, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, reasons.FailedGetResourceMetric, late.Findings[0].Reason)
}

// minReplicas defaults to 1, so an autoscaler without one and with
// maxReplicas 1 is pinned too.
func TestHPAMaxedOutSkipsPinnedAutoscalerWithDefaultFloor(t *testing.T) {
	model := newTestModel()
	id := newID(kube.KindHPA, "default", "app")
	put(model, id, t0, map[string]inventory.Value{
		kube.AttrCurrentReplicas: inventory.Number(1),
		kube.AttrDesiredReplicas: inventory.Number(1),
		kube.AttrMaxReplicas:     inventory.Number(1),
	})
	setCondition(model, id, "ScalingLimited", "True", "TooManyReplicas",
		"at the maximum", t0)

	got := evaluate(HPA{}, model, t0.Add(time.Minute), id, nil)

	assert.Empty(t, got.Findings)
}
