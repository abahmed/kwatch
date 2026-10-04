package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// commitmentModel builds node n1 with the given allocatable memory and
// one pod per limit; a zero limit is a container without one.
func commitmentModel(allocatable float64, limits ...float64) (
	*inventory.Model, inventory.EntityID,
) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]inventory.Value{
		kube.AttrMemoryAllocatable: inventory.Number(allocatable)})
	for i, limit := range limits {
		pod := newID(kube.KindPod, "shop", "api-"+string(rune('a'+i)))
		container := newID(kube.KindContainer, "shop", pod.Name+"/app")
		put(m, pod, t0, map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running")})
		attrs := map[string]inventory.Value{}
		if limit > 0 {
			attrs[kube.AttrMemoryLimit] = inventory.Number(limit)
		}
		put(m, container, t0, attrs)
		relateEntity(m, pod, inventory.RunsOn, node)
		relateEntity(m, container, inventory.PartOf, pod)
	}
	return m, node
}

func TestNodeCommitmentReportsMemoryLimitsFarAboveAllocatable(t *testing.T) {
	m, node := commitmentModel(1000, 800, 800, 0)

	got := NodeCommitment{}.Detect(testDetectorContext(m, t0), entityOf(m, node))

	require.Len(t, got, 1)
	assert.Equal(t, "NodeMemoryOvercommitted", got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "160% of its memory")
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "containers without a memory limit", Value: "1"})
}

func TestNodeCommitmentStaysQuietWithinTheNodesMemory(t *testing.T) {
	m, node := commitmentModel(1000, 600, 500)

	got := NodeCommitment{}.Detect(testDetectorContext(m, t0), entityOf(m, node))

	assert.Empty(t, got)
}

// Pods that finished no longer hold memory and do not count.
func TestNodeCommitmentIgnoresFinishedPods(t *testing.T) {
	m, node := commitmentModel(1000, 900, 900)
	done := newID(kube.KindPod, "shop", "api-a")
	put(m, done, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Succeeded")})

	got := NodeCommitment{}.Detect(testDetectorContext(m, t0), entityOf(m, node))

	assert.Empty(t, got)
}
