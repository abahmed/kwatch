package detectors

import (
	"testing"
	"time"

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

// underMemoryPressure makes the kubelet report MemoryPressure on node:
// the present failure that gives the commitment something to explain.
func underMemoryPressure(m *inventory.Model, node inventory.EntityID) {
	put(m, node, t0, conditionAttrs(map[string]inventory.Value{
		kube.AttrMemoryAllocatable: inventory.Number(1000)},
		"MemoryPressure", "True", "KubeletHasInsufficientMemory", t0))
}

// sustainedEval evaluates the node at t0 and again overcommitSustain
// later, which is when a steady overcommit is raised.
func sustainedEval(
	d detection.Detector, m *inventory.Model, node inventory.EntityID,
) detection.Evaluation {
	registry := detection.NewRegistry(nil, d)
	registry.Evaluate(m, t0, node)
	return registry.Evaluate(m, t0.Add(overcommitSustain), node)
}

func TestNodeCommitmentReportsMemoryLimitsFarAboveAllocatable(t *testing.T) {
	m, node := commitmentModel(1000, 800, 800, 0)
	underMemoryPressure(m, node)

	got := sustainedEval(NodeCommitment{}, m, node).Findings

	require.Len(t, got, 1)
	assert.Equal(t, "NodeMemoryOvercommitted", got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "160% of its memory")
	assert.Contains(t, got[0].Summary, "is short on memory and its pods")
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "containers without a memory limit", Value: "1"})
}

// Limits above the node's memory are normal for burstable pods: with no
// pressure, eviction or kernel OOM on the node, nothing is reported, so
// no incident opens for a risk.
func TestNodeCommitmentStaysQuietWhileNothingIsShort(t *testing.T) {
	m, node := commitmentModel(1000, 800, 800, 0)

	assert.Empty(t, sustainedEval(NodeCommitment{}, m, node).Findings)
}

// The kernel's own OOM event on the node is a present failure too.
func TestNodeCommitmentFollowsAKernelOOMOnTheNode(t *testing.T) {
	m, node := commitmentModel(1000, 800, 800)
	registry := detection.NewRegistry(nil, NodeCommitment{})
	registry.Evaluate(m, t0, node)
	at := t0.Add(overcommitSustain)
	noteEntity(m, node, "SystemOOM", "System OOM encountered", 1, at)

	assert.Len(t, registry.Evaluate(m, at, node).Findings, 1)
}

// A share that crosses the limit for one evaluation is not raised.
func TestNodeCommitmentWaitsForTheShareToHold(t *testing.T) {
	m, node := commitmentModel(1000, 800, 800)
	underMemoryPressure(m, node)
	registry := detection.NewRegistry(nil, NodeCommitment{})

	first := registry.Evaluate(m, t0, node)
	assert.Empty(t, first.Findings)
	assert.Equal(t, overcommitSustain, first.RecheckAfter)
	later := registry.Evaluate(m, t0.Add(overcommitSustain-time.Second), node)
	assert.Empty(t, later.Findings)
}

// A raised finding holds while the share hovers just under the limit.
func TestNodeCommitmentHoldsWhileTheShareHovers(t *testing.T) {
	m, node := commitmentModel(1000, 800, 800)
	underMemoryPressure(m, node)
	registry := detection.NewRegistry(nil, NodeCommitment{})
	registry.Evaluate(m, t0, node)
	at := t0.Add(overcommitSustain)
	require.Len(t, registry.Evaluate(m, at, node).Findings, 1)

	put(m, newID(kube.KindContainer, "shop", "api-b/app"), t0,
		map[string]inventory.Value{kube.AttrMemoryLimit: inventory.Number(700)})
	at = at.Add(time.Minute)
	assert.Len(t, registry.Evaluate(m, at, node).Findings, 1,
		"150% fell to 145%: still raised")

	put(m, newID(kube.KindContainer, "shop", "api-b/app"), t0,
		map[string]inventory.Value{kube.AttrMemoryLimit: inventory.Number(500)})
	at = at.Add(time.Minute)
	assert.Empty(t, registry.Evaluate(m, at, node).Findings)
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

// A regular init container runs to completion before the app starts, so
// its limit reserves nothing; NodeHealth already leaves it out.
func TestNodeCommitmentIgnoresInitContainers(t *testing.T) {
	m, node := commitmentModel(1000, 700)
	pod := newID(kube.KindPod, "shop", "api-a")
	setup := newID(kube.KindContainer, "shop", "api-a/setup")
	put(m, setup, t0, map[string]inventory.Value{
		kube.AttrInit:        inventory.Bool(true),
		kube.AttrMemoryLimit: inventory.Number(900),
	})
	relateEntity(m, setup, inventory.PartOf, pod)

	got := NodeCommitment{}.Detect(testDetectorContext(m, t0), entityOf(m, node))

	assert.Empty(t, got)
}
