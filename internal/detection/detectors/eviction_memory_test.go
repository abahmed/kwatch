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

// evictedOn builds pod shop/api evicted a minute ago on node n1, which
// is (or is not) under memory pressure, next to a big importer.
func evictedOn(pressure string) (*inventory.Model, inventory.Entity) {
	model, pod, _ := failedPodRun(time.Minute, reasons.Evicted)
	node := inventory.CoreID(kube.KindNode, "", "n1")
	attrs := map[string]inventory.Value{
		kube.AttrMemoryAllocatable: inventory.Number(8 * 1024 * mib),
	}
	conditionAttrs(attrs, "MemoryPressure", pressure, "Test",
		ageNow.Add(-time.Hour))
	put(model, node, ageNow, attrs)
	link(model, pod.ID, inventory.RunsOn, node)
	c := newID(kube.KindContainer, "ns", "p/c")
	put(model, c, ageNow, map[string]inventory.Value{
		kube.AttrMemoryPeak24h: inventory.Number(900 * mib),
		kube.AttrMemoryReq:     inventory.Number(256 * mib),
	})
	addPodOn(model, node, "batch", "importer", "main", 3174*mib, 0,
		map[string]inventory.Value{
			kube.AttrMemoryReq: inventory.Number(1024 * mib)})
	p, _ := model.Entity(pod.ID)
	return model, p
}

func TestMemoryEvictionStatesNodeContext(t *testing.T) {
	model, pod := evictedOn("True")
	got := NewPod(PodThresholds{}).Detect(
		testDetectorContext(model, ageNow), pod)
	require.Len(t, got, 1)
	f := got[0]
	assert.Equal(t, "the node's memory",
		evidenceOf(f, detection.EvidenceOOMKind))
	assert.Equal(t, "used 900Mi, over its 256Mi request",
		evidenceOf(f, detection.EvidenceOverRequest))
	assert.Equal(t, "requested 1Gi of 8Gi allocatable; 3.1Gi in use",
		evidenceOf(f, detection.EvidenceNodeMemory))
	assert.Equal(t, []string{"batch/importer 3.1Gi (request 1Gi, " +
		"no limit)"}, usersOf(f))
}

func TestEvictionWithoutMemoryPressureAddsNoMemoryContext(t *testing.T) {
	model, pod := evictedOn("False")
	got := NewPod(PodThresholds{}).Detect(
		testDetectorContext(model, ageNow), pod)
	require.Len(t, got, 1)
	assert.Empty(t, evidenceOf(got[0], detection.EvidenceOOMKind))
	assert.Empty(t, usersOf(got[0]))
}
