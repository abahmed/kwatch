package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A workload short of ready replicas under an autoscaler at its ceiling
// is reported after CeilingUnavailable, not the full wait.
func TestWorkloadUnavailableUnderCeilingWaitsLess(t *testing.T) {
	model, hpa := limitedHPA(1, 4)
	dep := newID(kube.KindDeployment, "default", "app")
	put(model, dep, t0, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(4),
		kube.AttrReadyReplicas: inventory.Number(2),
	})
	link(model, hpa, inventory.Scales, dep)
	d := NewWorkload(5 * time.Minute)

	early := evaluate(d, model, t0.Add(CeilingUnavailable-time.Second),
		dep, nil)
	assert.Empty(t, early.Findings)
	late := evaluate(d, model, t0.Add(CeilingUnavailable), dep, nil)
	assert.Len(t, late.Findings, 1)
}

func TestWorkloadUnavailableWithoutCeilingWaitsFully(t *testing.T) {
	model := newTestModel()
	dep := newID(kube.KindDeployment, "default", "app")
	put(model, dep, t0, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(4),
		kube.AttrReadyReplicas: inventory.Number(2),
	})
	d := NewWorkload(5 * time.Minute)
	got := evaluate(d, model, t0.Add(CeilingUnavailable), dep, nil)
	assert.Empty(t, got.Findings)
}
