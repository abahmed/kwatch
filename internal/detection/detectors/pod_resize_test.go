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

func detectResize(
	conditionType, status, reason string, age time.Duration,
) ([]detection.Finding, time.Duration) {
	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "app"}
	attrs := conditionAttrs(map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running"),
		kube.AttrReady: inventory.Bool(true),
	}, conditionType, status, reason, podNodeNow.Add(-age))
	observeEntity(model, id, podNodeNow, attrs)
	registry := detection.NewRegistry(nil, NewPod(PodThresholds{}))
	evaluation := registry.Evaluate(model, podNodeNow, id)
	return evaluation.Findings, evaluation.RecheckAfter
}

func TestPodResizeFindings(t *testing.T) {
	cases := []struct {
		name, condition, reason string
		age                     time.Duration
		want, mode              string
	}{
		{"infeasible", "PodResizePending", "Infeasible", time.Second,
			reasons.PodResizeInfeasible, "Resize.Infeasible"},
		{"deferred too long", "PodResizePending", "Deferred",
			10 * time.Minute, reasons.PodResizeDeferred, "Resize.Deferred"},
		{"apply error", "PodResizeInProgress", "Error", time.Second,
			reasons.PodResizeError, "Resize.Error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, _ := detectResize(tc.condition, "True", tc.reason, tc.age)
			if assert.Len(t, found, 1) {
				assert.Equal(t, tc.want, found[0].Reason)
				assert.Equal(t, tc.mode, string(found[0].Mode))
				assert.Equal(t, detection.Degraded, found[0].Health)
			}
		})
	}
}

func TestPodResizeNotReported(t *testing.T) {
	found, recheck := detectResize("PodResizePending", "True", "Deferred",
		time.Minute)
	assert.Empty(t, found, "a recent deferral is retried by the kubelet")
	assert.Equal(t, 4*time.Minute, recheck)

	found, _ = detectResize("PodResizeInProgress", "True", "", time.Hour)
	assert.Empty(t, found, "a resize in progress is not a failure")

	found, _ = detectResize("PodResizePending", "False", "Infeasible",
		time.Hour)
	assert.Empty(t, found)
}
