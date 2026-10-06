package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestFitResourceNearMissNamesBestNodePerPool(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "a", 4000, nil)
	fitNodeIn(m, "n3", "general", "a", 4000, nil)
	fitNodeIn(m, "big1", "bulk", "a", 8000, nil)
	capPod(m, "x", "n1", "Running", 3000, 0)
	capPod(m, "y", "n3", "Running", 2800, 0)
	capPod(m, "z", "big1", "Running", 7000, 0)
	pod := fitPending(m, "p", 1500, kube.SchedulingSpec{})
	got := fitLines(fitOf(t, m, pod), detection.EvidenceFit)
	assert.Equal(t, []string{
		"pool general has no node with 1.5 CPU free " +
			"(best: n3 has 1.2 CPU free)",
		"pool bulk has no node with 1.5 CPU free " +
			"(best: big1 has 1 CPU free)",
	}, got)
}

func TestFitSinglePoolShortOfCPUIsLeftToTheNumbers(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "", "a", 4000, nil)
	capPod(m, "x", "n1", "Running", 3000, 0)
	pod := fitPending(m, "p", 2000, kube.SchedulingSpec{})
	assert.Empty(t, fitOf(t, m, pod))
}

func TestFitTaintNotTolerated(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "g1", "general", "a", 1000, nil)
	capPod(m, "x", "g1", "Running", 900, 0)
	fitNodeIn(m, "gpu-1", "gpu", "a", 8000, map[string]inventory.Value{
		kube.AttrTaints: inventory.Text("nvidia.com/gpu=true:NoSchedule")})
	pod := fitPending(m, "p", 2000, kube.SchedulingSpec{})
	ev := fitOf(t, m, pod)
	assert.Equal(t, []string{
		"pool gpu has room but taint nvidia.com/gpu=true:NoSchedule " +
			"isn't tolerated",
		"pool general has no node with 2 CPU free " +
			"(best: g1 has 100m CPU free)",
	}, fitLines(ev, detection.EvidenceFit))
	assert.Equal(t, []string{"tolerating nvidia.com/gpu=true:NoSchedule " +
		"would fit it on gpu-1"}, fitLines(ev, detection.EvidenceFitWould))
}

func TestFitTolerationLiftsTaint(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "gpu-1", "gpu", "a", 8000, map[string]inventory.Value{
		kube.AttrTaints: inventory.Text("gpu=true:NoSchedule")})
	fitNodeIn(m, "g1", "general", "a", 100, nil)
	spec := kube.SchedulingSpec{Tolerations: []kube.Toleration{
		{Key: "gpu", Operator: "Exists"}}}
	pod := fitPending(m, "p", 2000, spec)
	ev := fitOf(t, m, pod)
	assert.Empty(t, fitLines(ev, detection.EvidenceFitWould))
	assert.Empty(t, ev)
}

func TestFitPreferNoScheduleDoesNotBlock(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, map[string]inventory.Value{
		kube.AttrTaints: inventory.Text("soft=1:PreferNoSchedule")})
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	assert.Empty(t, fitOf(t, m, pod))
}

func TestFitCordonedAndNotReadyNodes(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "c1", "general", "a", 4000, map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true),
		kube.AttrTaints: inventory.Text(
			"node.kubernetes.io/unschedulable:NoSchedule")})
	fitNodeIn(m, "d1", "other", "a", 4000, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	got := fitLines(fitOf(t, m, pod), detection.EvidenceFit)
	assert.Equal(t, []string{
		"pool general: the closest node, c1, is cordoned",
		"pool other: the closest node, d1, is NotReady"}, got)
}

func TestFitNodeSelectorMissingEverywhere(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, nil)
	fitNodeIn(m, "n2", "b", "z", 4000, nil)
	spec := kube.SchedulingSpec{Selector: map[string]string{
		"disktype": "ssd"}}
	pod := fitPending(m, "p", 1000, spec)
	ev := fitOf(t, m, pod)
	assert.Equal(t, []string{
		"no node has the label disktype=ssd the pod selects"},
		fitLines(ev, detection.EvidenceFit))
}

func TestFitNodeSelectorMatchesOnePool(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, nil)
	fitNodeIn(m, "s1", "ssd", "z", 500, map[string]inventory.Value{
		kube.AttrNodeLabels: inventory.Text("disktype=ssd")})
	spec := kube.SchedulingSpec{Selector: map[string]string{
		"disktype": "ssd"}}
	pod := fitPending(m, "p", 1000, spec)
	ev := fitOf(t, m, pod)
	assert.Equal(t, []string{
		"pool a: the closest node, n1, lacks the label disktype=ssd " +
			"the pod selects",
		"pool ssd has no node with 1 CPU free (best: s1 has 500m CPU free)",
	}, fitLines(ev, detection.EvidenceFit))
	assert.Equal(t, []string{"dropping the selector disktype=ssd would " +
		"fit it on n1"}, fitLines(ev, detection.EvidenceFitWould))
}

func TestFitNodeAffinityTerms(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, nil)
	fitNodeIn(m, "n2", "b", "z", 4000, map[string]inventory.Value{
		kube.AttrNodeLabels: inventory.Text("tier=gold")})
	spec := kube.SchedulingSpec{NodeTerms: [][]kube.Requirement{
		{{Key: "tier", Op: "In", Values: []string{"gold", "silver"}}}}}
	pod := fitPending(m, "p", 1000, spec)
	assert.Empty(t, fitOf(t, m, pod))
	spec.NodeTerms = [][]kube.Requirement{
		{{Key: "tier", Op: "In", Values: []string{"platinum"}}}}
	pod = fitPending(m, "q", 1000, spec)
	assert.Equal(t, []string{
		"no node meets the pod's node affinity (tier=platinum)"},
		fitLines(fitOf(t, m, pod), detection.EvidenceFit))
}

func TestFitNodeWithUnreadLabelsIsNotJudged(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, map[string]inventory.Value{
		kube.AttrNodeLabelsCut: inventory.Bool(true)})
	spec := kube.SchedulingSpec{Selector: map[string]string{"x": "y"}}
	pod := fitPending(m, "p", 1000, spec)
	assert.Empty(t, fitOf(t, m, pod))
}

func TestFitExtendedResource(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "a", 4000, nil)
	fitNodeIn(m, "g1", "gpu", "a", 4000, map[string]inventory.Value{
		kube.AttrAllocatable: inventory.Text("nvidia.com/gpu=2")})
	fitRunning(m, "r1", "g1", "", 100)
	cid := kube.ContainerID("d", "r1", "app")
	observeEntity(m, cid, podNodeNow, map[string]inventory.Value{
		kube.AttrExtendedReq: inventory.Text("nvidia.com/gpu=2")})
	pod := fitPending(m, "p", 100, kube.SchedulingSpec{})
	observeEntity(m, kube.ContainerID("d", "p", "app"), podNodeNow,
		map[string]inventory.Value{
			kube.AttrCPUReq:      inventory.Number(100),
			kube.AttrExtendedReq: inventory.Text("nvidia.com/gpu=1"),
		})
	got := fitLines(fitOf(t, m, pod), detection.EvidenceFit)
	assert.Equal(t, []string{
		"pool general has no node with 1 nvidia.com/gpu free " +
			"(best: n1 has 0 nvidia.com/gpu free)",
		"pool gpu has no node with 1 nvidia.com/gpu free " +
			"(best: g1 has 0 nvidia.com/gpu free)"}, got)
}

func TestFitPodSlotsAndEphemeralStorage(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, map[string]inventory.Value{
		kube.AttrAllocatable: inventory.Text(
			"pods=1,ephemeral-storage=10737418240")})
	fitNodeIn(m, "n2", "b", "z", 4000, map[string]inventory.Value{
		kube.AttrAllocatable: inventory.Text(
			"pods=10,ephemeral-storage=10737418240")})
	fitRunning(m, "r1", "n1", "", 10)
	fitRunning(m, "r2", "n2", "", 10)
	pod := fitPending(m, "p", 100, kube.SchedulingSpec{})
	observeEntity(m, kube.ContainerID("d", "p", "app"), podNodeNow,
		map[string]inventory.Value{
			kube.AttrCPUReq:       inventory.Number(100),
			kube.AttrEphemeralReq: inventory.Number(20 * gib),
		})
	got := fitLines(fitOf(t, m, pod), detection.EvidenceFit)
	assert.Equal(t, []string{
		"pool b has no node with 20 GiB ephemeral storage free " +
			"(best: n2 has 10 GiB ephemeral storage free)",
		"pool a has no node with 20 GiB ephemeral storage and 1 pod " +
			"slot free (best: n1 has 10 GiB ephemeral storage and 0 " +
			"pod slots free)"}, got)
}

func TestFitNodesCappedAndNoteSaysSo(t *testing.T) {
	m := newTestModel()
	for i := 0; i < maxFitNodes+5; i++ {
		fitNodeIn(m, "n"+string(rune('a'+i/26))+string(rune('a'+i%26)),
			"p"+string(rune('a'+i%2)), "z", 100, nil)
	}
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	notes := fitLines(fitOf(t, m, pod), detection.EvidenceFitNote)
	assert.Equal(t, []string{"checked 200 of 205 nodes"}, notes)
}

func TestFitTooBigSpecIsSkipped(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 100, nil)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{TooBig: true})
	ev := fitOf(t, m, pod)
	assert.Equal(t, []detection.Evidence{{Label: detection.EvidenceFitNote,
		Value: "the pod's scheduling rules are too large for kwatch " +
			"to check"}}, ev)
}

func TestFitEveryNodePassesButSchedulerRefuses(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, nil)
	fitNodeIn(m, "n2", "a", "z", 4000, nil)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	assert.Empty(t, fitOf(t, m, pod))
}

func TestFitDoesNotClaimRoomForAShortageItCannotMeasure(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, nil)
	pod := fitPending(m, "p", 0, kube.SchedulingSpec{})
	since := podNodeNow.Add(-10 * time.Minute)
	cond := kube.ConditionKey("PodScheduled")
	observeEntity(m, pod, since, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
		cond:           inventory.Text("False"),
		cond + kube.AttrConditionMessage: inventory.Text(
			"0/1 nodes are available: 1 Insufficient memory."),
	})
	assert.Empty(t, fitOf(t, m, pod))
}
