package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func cpuEntity(attrs map[string]float64) inventory.Entity {
	m := newTestModel()
	id := kube.ContainerID("shop", "api-1", "app")
	values := map[string]inventory.Value{}
	for name, v := range attrs {
		values[name] = inventory.Number(v)
	}
	put(m, id, t0, values)
	return entityOf(m, id)
}

func TestThrottleEvidenceAtLimitWithoutHeavyThrottling(t *testing.T) {
	got := throttleEvidence(cpuEntity(map[string]float64{
		kube.AttrThrottledPct: 20, kube.AttrCPUUsageMilli: 248,
		kube.AttrCPULimit: 250, kube.AttrCPUPeak24h: 250,
	}))
	f := detection.Finding{Evidence: got}
	assert.Equal(t, "248m", evidenceOf(f, detection.EvidenceCPUAtLimit))
	assert.Equal(t, "250m", evidenceOf(f, detection.EvidenceCPULimit))
	assert.Equal(t, "250m", evidenceOf(f, detection.EvidenceCPUPeak))
	assert.Empty(t, evidenceOf(f, detection.EvidenceCPUThrottled))
}

func TestThrottleEvidenceNothingWhenNotAtLimit(t *testing.T) {
	for name, attrs := range map[string]map[string]float64{
		"well under the limit": {kube.AttrThrottledPct: 5,
			kube.AttrCPUUsageMilli: 100, kube.AttrCPULimit: 250},
		"no limit":   {kube.AttrCPUUsageMilli: 900},
		"no reading": {kube.AttrCPULimit: 250},
	} {
		assert.Empty(t, throttleEvidence(cpuEntity(attrs)), name)
	}
}
