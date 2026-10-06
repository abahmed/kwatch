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

func detectHPA(
	t *testing.T, conditions map[string][2]string,
) []detection.Finding {
	t.Helper()
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindHPA, Namespace: "default",
		Name: "app"}
	model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: now, Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrCurrentReplicas: inventory.Number(1),
		},
	})
	for condType, statusReason := range conditions {
		setCondition(model, id, condType, statusReason[0], statusReason[1],
			"condition message", now)
	}
	entity, _ := model.Entity(id)
	// Conditions are reported once they outlast the grace period; a
	// metrics failure has the longer one.
	later := now.Add(HPAMetricsGrace)
	return HPA{}.Detect(testDetectorContext(model, later), entity)
}

func TestHPAConditionReasons(t *testing.T) {
	tt := []struct {
		name       string
		conditions map[string][2]string
		want       []string
	}{
		{
			name: "external_metric_failure_folds_into_metrics_reason",
			conditions: map[string][2]string{
				"ScalingActive": {"False", "FailedGetExternalMetric"}},
			want: []string{reasons.FailedGetResourceMetric},
		},
		{
			name: "scale_target_read_failure",
			conditions: map[string][2]string{
				"ScalingActive": {"False", "FailedGetScale"}},
			want: []string{reasons.FailedGetScale},
		},
		{
			name: "selector_failure",
			conditions: map[string][2]string{
				"ScalingActive": {"False", "InvalidSelector"}},
			want: []string{reasons.HPAInvalidSelector},
		},
		{
			name: "able_to_scale_update_failure",
			conditions: map[string][2]string{
				"AbleToScale": {"False", "FailedUpdateScale"}},
			want: []string{reasons.FailedUpdateScale},
		},
		{
			name: "same_reason_on_both_conditions_reports_once",
			conditions: map[string][2]string{
				"ScalingActive": {"False", "FailedGetScale"},
				"AbleToScale":   {"False", "FailedGetScale"}},
			want: []string{reasons.FailedGetScale},
		},
		{
			name: "unknown_reason_is_generic",
			conditions: map[string][2]string{
				"ScalingActive": {"False", "SomethingNew"}},
			want: []string{reasons.HPAScalingError},
		},
		{
			name: "scaling_disabled_is_not_a_failure",
			conditions: map[string][2]string{
				"ScalingActive": {"False", "ScalingDisabled"}},
			want: nil,
		},
		{
			name: "healthy_conditions_are_silent",
			conditions: map[string][2]string{
				"ScalingActive": {"True", "ValidMetricFound"},
				"AbleToScale":   {"True", "ReadyForNewScale"}},
			want: nil,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, f := range detectHPA(t, tc.conditions) {
				got = append(got, f.Reason)
			}
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}

func TestHPAConditionFindingCarriesMessage(t *testing.T) {
	findings := detectHPA(t, map[string][2]string{
		"ScalingActive": {"False", "FailedGetResourceMetric"}})
	require.Len(t, findings, 1)
	assert.Equal(t, "condition message", findings[0].Evidence[0].Value)
}

// TestHPAConditionWaitsForGrace: a False condition is reported only
// after it has lasted DefaultConditionGrace, as for other conditions.
func TestHPAConditionWaitsForGrace(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindHPA, "default", "app")
	put(m, id, t0, nil)
	setCondition(m, id, "ScalingActive", "False", "FailedGetScale",
		"cannot read scale", t0)

	early := evaluate(HPA{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultConditionGrace-time.Minute, early.RecheckAfter)

	late := evaluate(HPA{}, m, t0.Add(DefaultConditionGrace), id, nil)
	require.Len(t, late.Findings, 1)
}
