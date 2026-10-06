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

const gib = float64(1 << 30)

func capNode(
	m *inventory.Model, name string, cpu, mem float64,
	extra map[string]inventory.Value,
) inventory.EntityID {
	id := inventory.EntityID{Kind: kube.KindNode, Name: name}
	attrs := map[string]inventory.Value{
		kube.AttrCPUAllocatable:    inventory.Number(cpu),
		kube.AttrMemoryAllocatable: inventory.Number(mem),
		kube.AttrReady:             inventory.Bool(true),
	}
	for k, v := range extra {
		attrs[k] = v
	}
	observeEntity(m, id, podNodeNow, attrs)
	return id
}

// capPod adds a pod with one container asking cpu/mem; node may be "".
func capPod(
	m *inventory.Model, name, node, phase string, cpu, mem float64,
) inventory.EntityID {
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "d", Name: name}
	observeEntity(m, id, podNodeNow, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text(phase)})
	cid := kube.ContainerID("d", name, "app")
	attrs := map[string]inventory.Value{}
	if cpu > 0 {
		attrs[kube.AttrCPUReq] = inventory.Number(cpu)
	}
	if mem > 0 {
		attrs[kube.AttrMemoryReq] = inventory.Number(mem)
	}
	observeEntity(m, cid, podNodeNow, attrs)
	relateEntity(m, cid, inventory.PartOf, id)
	if node != "" {
		relateEntity(m, id, inventory.RunsOn,
			inventory.EntityID{Kind: kube.KindNode, Name: node})
	}
	return id
}

func evidenceMap(ev []detection.Evidence) map[string][]string {
	out := map[string][]string{}
	for _, e := range ev {
		out[e.Label] = append(out[e.Label], e.Value)
	}
	return out
}

func quantified(
	t *testing.T, m *inventory.Model, pod inventory.EntityID, msg string,
) []detection.Evidence {
	t.Helper()
	since := podNodeNow.Add(-10 * time.Minute)
	cond := kube.ConditionKey("PodScheduled")
	observeEntity(m, pod, since, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
		cond:           inventory.Text("False"),
		cond + kube.AttrConditionReason: inventory.Text(
			"Unschedulable"),
		cond + kube.AttrConditionMessage: inventory.Text(msg),
		cond + kube.AttrConditionSince:   inventory.Time(since),
	})
	entity, _ := m.Entity(pod)
	found := NewPod(PodThresholds{}).Detect(
		testDetectorContext(m, podNodeNow), entity)
	require.Len(t, found, 1)
	return found[0].Evidence
}

const cpuMsg = "0/3 nodes are available: 3 Insufficient cpu."
const memMsg = "0/3 nodes are available: 3 Insufficient memory."

func TestUnschedulableInsufficientCPU(t *testing.T) {
	m := newTestModel()
	capNode(m, "n1", 4000, 16*gib, nil)
	capNode(m, "n2", 4000, 16*gib, nil)
	capPod(m, "a", "n1", "Running", 2500, 0)
	capPod(m, "b", "n2", "Running", 3000, 0)
	pod := capPod(m, "big", "", "Pending", 2000, 0)
	got := evidenceMap(quantified(t, m, pod, cpuMsg))
	assert.Equal(t, []string{"2 CPU"}, got["needs"])
	assert.Equal(t, []string{"1.5 CPU on n1"}, got["most free"])
}

func TestUnschedulableInsufficientMemory(t *testing.T) {
	m := newTestModel()
	capNode(m, "n1", 4000, 16*gib, nil)
	capPod(m, "a", "n1", "Running", 0, 12*gib)
	pod := capPod(m, "big", "", "Pending", 0, 4*gib)
	got := evidenceMap(quantified(t, m, pod, memMsg))
	assert.Equal(t, []string{"4 GiB memory"}, got["needs"])
	assert.Equal(t, []string{"4 GiB memory on n1"}, got["most free"])
}

func TestUnschedulableBothResources(t *testing.T) {
	m := newTestModel()
	capNode(m, "n1", 1000, 2*gib, nil)
	pod := capPod(m, "big", "", "Pending", 250, 3*gib)
	msg := "0/1 nodes are available: 1 Insufficient cpu, " +
		"1 Insufficient memory."
	got := evidenceMap(quantified(t, m, pod, msg))
	assert.Equal(t, []string{"250m CPU", "3 GiB memory"}, got["needs"])
	assert.Equal(t, []string{"1 CPU on n1", "2 GiB memory on n1"},
		got["most free"])
}

func TestUnschedulableSkipsCordonedAndNotReady(t *testing.T) {
	m := newTestModel()
	capNode(m, "ok", 4000, 16*gib, nil)
	capNode(m, "cordoned", 64000, 16*gib, map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	capNode(m, "down", 64000, 16*gib, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	capPod(m, "a", "ok", "Running", 3000, 0)
	pod := capPod(m, "big", "", "Pending", 2000, 0)
	got := evidenceMap(quantified(t, m, pod, cpuMsg))
	assert.Equal(t, []string{"1 CPU on ok"}, got["most free"])
}

func TestUnschedulableNoSchedulableNode(t *testing.T) {
	m := newTestModel()
	capNode(m, "c", 4000, 16*gib, map[string]inventory.Value{
		kube.AttrUnschedulable: inventory.Bool(true)})
	pod := capPod(m, "big", "", "Pending", 2000, 0)
	got := evidenceMap(quantified(t, m, pod, cpuMsg))
	assert.Equal(t, []string{"no schedulable node"}, got["most free"])
}

func TestUnschedulableIgnoresFinishedPods(t *testing.T) {
	m := newTestModel()
	capNode(m, "n1", 4000, 16*gib, nil)
	capPod(m, "done", "n1", "Succeeded", 3000, 0)
	capPod(m, "dead", "n1", "Failed", 500, 0)
	pod := capPod(m, "big", "", "Pending", 6000, 0)
	got := evidenceMap(quantified(t, m, pod, cpuMsg))
	assert.Equal(t, []string{"4 CPU on n1"}, got["most free"])
}

func TestUnschedulableNoRequestsNoNumbers(t *testing.T) {
	m := newTestModel()
	capNode(m, "n1", 4000, 16*gib, nil)
	pod := capPod(m, "big", "", "Pending", 0, 0)
	got := evidenceMap(quantified(t, m, pod, cpuMsg))
	assert.NotContains(t, got, "needs")
	assert.NotContains(t, got, "most free")
}

func TestUnschedulableTaintOnlyNoNumbers(t *testing.T) {
	m := newTestModel()
	capNode(m, "n1", 4000, 16*gib, nil)
	pod := capPod(m, "big", "", "Pending", 2000, 0)
	msg := "0/1 nodes are available: 1 node(s) had untolerated taint " +
		"{dedicated: gpu}."
	got := evidenceMap(quantified(t, m, pod, msg))
	assert.NotContains(t, got, "needs")
}
