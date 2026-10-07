package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestBaselineSamplerRecordsStartFromContainerStart: the start sample
// counts from the newest container start, not from the pod's creation,
// so scheduling and image pulls do not inflate it.
func TestBaselineSamplerRecordsStartFromContainerStart(t *testing.T) {
	r := newSamplerRig(t)
	r.observe(r.pod, map[string]inventory.Value{
		kube.AttrCreated:   inventory.Time(timelineAt(0)),
		kube.AttrStartTime: inventory.Time(timelineAt(time.Second)),
		kube.AttrContainersStarted: inventory.Time(
			timelineAt(40 * time.Second)),
		kube.AttrReady: inventory.Bool(true),
		kube.AttrReadySince: inventory.Time(
			timelineAt(115 * time.Second)),
	})
	start := r.stat(inventory.MetricStartSeconds)
	assert.Equal(t, []float64{75}, start.Recent)
	ready := r.stat(inventory.MetricReadySeconds)
	assert.Equal(t, []float64{115}, ready.Recent)
}

// TestBaselineSamplerFallsBackToPodStart: without a container start,
// the pod's start time is the reference.
func TestBaselineSamplerFallsBackToPodStart(t *testing.T) {
	r := newSamplerRig(t)
	r.observe(r.pod, map[string]inventory.Value{
		kube.AttrCreated:    inventory.Time(timelineAt(0)),
		kube.AttrStartTime:  inventory.Time(timelineAt(10 * time.Second)),
		kube.AttrReady:      inventory.Bool(true),
		kube.AttrReadySince: inventory.Time(timelineAt(70 * time.Second)),
	})
	start := r.stat(inventory.MetricStartSeconds)
	assert.Equal(t, []float64{60}, start.Recent)
}
