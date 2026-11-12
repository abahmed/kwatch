package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cappedNode is a 4-CPU node whose tasks stall on CPU half of the time,
// running one container at its 250m limit and one that uses busyMilli.
func cappedNode(busyMilli float64) (*inventory.Model, inventory.EntityID,
	inventory.EntityID,
) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]inventory.Value{
		kube.AttrCPUPSI:         inventory.Number(54),
		kube.AttrCPUAllocatable: inventory.Number(4000),
	})
	capped := addCPUContainer(m, node, "report", 249, 250)
	addCPUContainer(m, node, "web", busyMilli, 0)
	return m, node, capped
}

func addCPUContainer(m *inventory.Model, node inventory.EntityID,
	name string, used, limit float64,
) inventory.EntityID {
	pod := newID(kube.KindPod, "shop", name+"-0")
	put(m, pod, t0, nil)
	link(m, pod, inventory.RunsOn, node)
	id := kube.ContainerID("shop", pod.Name, "app")
	attrs := map[string]inventory.Value{
		kube.AttrCPUUsageMilli: inventory.Number(used)}
	if limit > 0 {
		attrs[kube.AttrCPULimit] = inventory.Number(limit)
	}
	put(m, id, t0, attrs)
	link(m, id, inventory.PartOf, pod)
	return id
}

// A node that uses a tenth of its CPU is not short of it: its stall is
// a container held at its own limit, and that container is reported.
func TestCPUStallFromCappedContainerIsNotNodePressure(t *testing.T) {
	m, node, capped := cappedNode(150)
	registry := detection.NewRegistry(nil, NodeUsage{})
	registry.Evaluate(m, t0, node)
	assert.Empty(t, registry.Evaluate(m, t0.Add(psiSustain), node).Findings)

	got := evaluateAfterGrace(ContainerResources{}, m, t0, psiSustain,
		capped, nil).Findings
	require.NotEmpty(t, got)
	f := got[len(got)-1]
	assert.Equal(t, reasons.ContainerCPUThrottled, f.Reason)
	assert.Equal(t, detection.Warning, f.Severity)
	assert.Contains(t, f.Summary, "249m of 250m")
}

// A node whose pods use nearly all of its CPU is short of it, even
// with a container at its limit.
func TestCPUStallOnBusyNodeIsNodePressure(t *testing.T) {
	m, node, _ := cappedNode(3600)
	registry := detection.NewRegistry(nil, NodeUsage{})
	registry.Evaluate(m, t0, node)
	got := registry.Evaluate(m, t0.Add(psiSustain), node).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.NodePSIHigh, got[0].Reason)
}

// Without a container at its limit the stall is the node's.
func TestCPUStallWithoutCappedContainerIsNodePressure(t *testing.T) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]inventory.Value{
		kube.AttrCPUPSI:         inventory.Number(54),
		kube.AttrCPUAllocatable: inventory.Number(4000),
	})
	addCPUContainer(m, node, "web", 150, 0)
	registry := detection.NewRegistry(nil, NodeUsage{})
	registry.Evaluate(m, t0, node)
	got := registry.Evaluate(m, t0.Add(psiSustain), node).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.NodePSIHigh, got[0].Reason)
}
