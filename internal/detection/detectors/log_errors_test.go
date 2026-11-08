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

func sampled(errors, baseline, span float64) (*inventory.Model,
	inventory.EntityID) {
	model := newTestModel()
	id := newID(kube.KindDeployment, "shop", "orders")
	put(model, id, t0, map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(3),
		kube.AttrReadyReplicas:   inventory.Number(3),
		kube.AttrLogErrors:       inventory.Number(errors),
		kube.AttrLogWindow:       inventory.Number(120),
		kube.AttrLogBaseline:     inventory.Number(baseline),
		kube.AttrLogBaselineSpan: inventory.Number(span),
		kube.AttrLogTopLine:      inventory.Text("pq: connection refused"),
		kube.AttrLogTopCount:     inventory.Number(errors),
		kube.AttrLogTopBaseline:  inventory.Number(baseline),
	})
	return model, id
}

func TestLogErrorsFlagsABurstNewAgainstTheBaseline(t *testing.T) {
	model, id := sampled(340, 7, 720)
	got := evaluate(LogErrors{}, model, t0.Add(time.Minute), id, nil)
	require.Len(t, got.Findings, 1)
	f := got.Findings[0]
	assert.Equal(t, reasons.LogErrorBurst, f.Reason)
	assert.Equal(t, detection.ModeErrorBurst, detection.ModeFor(f.Reason))
	assert.Equal(t, detection.Warning, f.Severity)
	assert.Equal(t, "340 errors in 2 min", f.Summary)
}

func TestLogErrorsJudgesAgainstTheWorkloadsOwnRate(t *testing.T) {
	tt := []struct {
		name                   string
		errors, baseline, span float64
		replicas, ready        float64
		want                   bool
	}{
		{"burst", 340, 7, 720, 3, 3, true},
		{"no_earlier_errors", 40, 0, 720, 3, 3, true},
		{"steady_noise", 330, 1800, 720, 3, 3, false},
		{"under_floor", 19, 0, 720, 3, 3, false},
		{"short_history", 400, 0, 240, 3, 3, false},
		{"kubernetes_sees_it", 340, 7, 720, 3, 2, false},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			model, id := sampled(tc.errors, tc.baseline, tc.span)
			put(model, id, t0, map[string]inventory.Value{
				kube.AttrReplicas:        inventory.Number(tc.replicas),
				kube.AttrReadyReplicas:   inventory.Number(tc.ready),
				kube.AttrLogErrors:       inventory.Number(tc.errors),
				kube.AttrLogWindow:       inventory.Number(120),
				kube.AttrLogBaseline:     inventory.Number(tc.baseline),
				kube.AttrLogBaselineSpan: inventory.Number(tc.span),
			})
			got := evaluate(LogErrors{}, model, t0.Add(time.Minute), id, nil)
			assert.Equal(t, tc.want, len(got.Findings) == 1)
		})
	}
}
