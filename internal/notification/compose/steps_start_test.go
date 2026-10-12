package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A container that never ran has no previous output, and an evicted pod
// was removed by the kubelet: both are read with describe.
func TestNextStepsDescribePodWhenNothingRanToLogOrWasEvicted(t *testing.T) {
	container := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	pod := inventory.CoreID(kube.KindPod, "shop", "api-1")
	tests := []struct {
		name     string
		findings []detection.Finding
		want     string
	}{
		{"image never pulled", []detection.Finding{{Entity: container,
			Reason: reasons.ErrImageNeverPull}},
			"kubectl describe pod api-1 -n shop"},
		{"config error", []detection.Finding{{Entity: container,
			Reason: reasons.CreateConfigError}},
			"kubectl describe pod api-1 -n shop"},
		{"evicted pod seen through its container", []detection.Finding{
			{Entity: container, Reason: "Error"},
			{Entity: pod, Reason: reasons.Evicted}},
			"kubectl describe pod api-1 -n shop"},
		{"evicted container finding", []detection.Finding{{
			Entity: container, Reason: reasons.Evicted}},
			"kubectl describe pod api-1 -n shop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := incident.Incident{Root: pod}

			steps := nextSteps(p, tt.findings)

			if len(steps) != 1 || steps[0].Command != tt.want {
				t.Fatalf("got %+v, want only %q", steps, tt.want)
			}
		})
	}
}
