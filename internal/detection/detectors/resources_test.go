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

func containerWith(attrs map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kube.KindContainer, "default", "web/app")
	put(m, id, t0, attrs)
	return m, id
}

func TestContainerResourcesMemory(t *testing.T) {
	tests := []struct {
		name     string
		used     float64
		fires    bool
		severity detection.Severity
	}{
		{"quiet below 90", 89, false, 0},
		{"warning at 90", 90, true, detection.Warning},
		{"critical at 95", 95, true, detection.Critical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, id := containerWith(map[string]inventory.Value{
				kube.AttrMemoryWorking: inventory.Number(tt.used),
				kube.AttrMemoryLimit:   inventory.Number(100),
			})
			got := evaluate(ContainerResources{}, m, t0, id, nil).Findings
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, reasons.ContainerMemoryHigh,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestContainerResourcesCPUSustained(t *testing.T) {
	m, id := containerWith(map[string]inventory.Value{
		kube.AttrCPUUsageMilli: inventory.Number(95),
		kube.AttrCPULimit:      inventory.Number(100),
	})
	early := evaluate(ContainerResources{}, m, t0.Add(4*time.Minute), id,
		nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, 6*time.Minute, early.RecheckAfter)

	late := evaluate(ContainerResources{}, m, t0.Add(10*time.Minute), id,
		nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, reasons.ContainerCPUHigh, late.Findings[0].Reason)
	assert.Zero(t, late.RecheckAfter)
}

func TestContainerResourcesThrottled(t *testing.T) {
	tests := []struct {
		name     string
		pct      float64
		fires    bool
		severity detection.Severity
	}{
		{"quiet below 50", 49, false, 0},
		{"warning at 50", 50, true, detection.Warning},
		{"critical at 75", 75, true, detection.Critical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, id := containerWith(map[string]inventory.Value{
				kube.AttrThrottledPct: inventory.Number(tt.pct),
			})
			now := t0.Add(11 * time.Minute)
			got := evaluate(ContainerResources{}, m, now, id, nil).Findings
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, reasons.ContainerCPUThrottled,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
		})
	}
}

func TestContainerResourcesThrottledPendingRecheck(t *testing.T) {
	m, id := containerWith(map[string]inventory.Value{
		kube.AttrThrottledPct: inventory.Number(80),
	})
	got := evaluate(ContainerResources{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, got.Findings)
	assert.Equal(t, 9*time.Minute, got.RecheckAfter)
	assert.Equal(t, "container-resources", ContainerResources{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindContainer},
		ContainerResources{}.Kinds())
}

func podWithContainers(limits ...float64,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	pod := newID(kube.KindPod, "default", "web")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrEphemeralUsed: inventory.Number(90),
	})
	for i, limit := range limits {
		c := newID(kube.KindContainer, "default",
			"web/c"+string(rune('a'+i)))
		put(m, c, t0, map[string]inventory.Value{
			kube.AttrEphemeralLimit: inventory.Number(limit),
		})
		link(m, c, inventory.PartOf, pod)
	}
	return m, pod
}

func TestPodStorageNearLimit(t *testing.T) {
	m, pod := podWithContainers(60, 40)
	got := evaluate(PodStorage{}, m, t0, pod, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ContainerEphemeralStorageHigh,
		got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.NotEmpty(t, got[0].Summary)
}

func TestPodStorageCritical(t *testing.T) {
	m, pod := podWithContainers(94)
	got := evaluate(PodStorage{}, m, t0, pod, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, detection.Critical, got[0].Severity)
}

func TestPodStorageQuiet(t *testing.T) {
	m, pod := podWithContainers(101)
	assert.Empty(t, evaluate(PodStorage{}, m, t0, pod, nil).Findings)

	m, pod = podWithContainers()
	assert.Empty(t, evaluate(PodStorage{}, m, t0, pod, nil).Findings,
		"no limit declared")

	m = newTestModel()
	bare := newID(kube.KindPod, "default", "bare")
	put(m, bare, t0, nil)
	assert.Empty(t, evaluate(PodStorage{}, m, t0, bare, nil).Findings,
		"no usage reported")
	assert.Equal(t, "pod-storage", PodStorage{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindPod}, PodStorage{}.Kinds())
}

func TestNodeHealthErrorRates(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrNetErrorRate:   inventory.Number(1),
		kube.AttrRuntimeErrRate: inventory.Number(10),
	})
	got := evaluate(NodeHealth{}, m, t0, id, nil).Findings
	require.Len(t, got, 2)
	assert.Equal(t, reasons.NodeNetworkErrors, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, reasons.NodeRuntimeErrors, got[1].Reason)
	assert.Equal(t, detection.Critical, got[1].Severity)
	assert.NotEmpty(t, got[0].Summary)
}

func TestNodeHealthQuietBelowOne(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrNetErrorRate: inventory.Number(0.9),
	})
	assert.Empty(t, evaluate(NodeHealth{}, m, t0, id, nil).Findings)
	assert.Equal(t, "node-health", NodeHealth{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindNode}, NodeHealth{}.Kinds())
}

func overcommittedNode(cpuLimit, memLimit float64,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]inventory.Value{
		kube.AttrCPUAllocatable:    inventory.Number(1000),
		kube.AttrMemoryAllocatable: inventory.Number(1000),
	})
	pod := newID(kube.KindPod, "default", "web")
	put(m, pod, t0, nil)
	link(m, pod, inventory.RunsOn, node)
	c := newID(kube.KindContainer, "default", "web/app")
	put(m, c, t0, map[string]inventory.Value{
		kube.AttrCPULimit:    inventory.Number(cpuLimit),
		kube.AttrMemoryLimit: inventory.Number(memLimit),
	})
	link(m, c, inventory.PartOf, pod)
	return m, node
}

func TestNodeHealthOvercommit(t *testing.T) {
	tests := []struct {
		name     string
		cpu, mem float64
		reason   string
		severity detection.Severity
		fires    bool
	}{
		{"quiet under 2x", 1999, 100, "", 0, false},
		{"high at 2x", 2000, 100, reasons.NodeResourceHigh,
			detection.Warning, true},
		{"critical at 4x memory", 100, 4000,
			reasons.NodeResourceCritical, detection.Critical, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, node := overcommittedNode(tt.cpu, tt.mem)
			got := evaluate(NodeHealth{}, m, t0, node, nil).Findings
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, tt.reason, got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestNodeHealthOvercommitNeedsAllocatable(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	put(m, id, t0, nil)
	assert.Empty(t, evaluate(NodeHealth{}, m, t0, id, nil).Findings)
}

// TestNodeHealthOvercommitCountsOnlyRunningAppLimits: finished pods and
// init containers hold no limits against the node.
func TestNodeHealthOvercommitCountsOnlyRunningAppLimits(t *testing.T) {
	m, node := overcommittedNode(100, 100)
	done := newID(kube.KindPod, "default", "job")
	put(m, done, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Succeeded"),
	})
	link(m, done, inventory.RunsOn, node)
	big := newID(kube.KindContainer, "default", "job/main")
	put(m, big, t0, map[string]inventory.Value{
		kube.AttrCPULimit: inventory.Number(9000),
	})
	link(m, big, inventory.PartOf, done)
	pod := newID(kube.KindPod, "default", "web")
	setup := newID(kube.KindContainer, "default", "web/setup")
	put(m, setup, t0, map[string]inventory.Value{
		kube.AttrInit:     inventory.Bool(true),
		kube.AttrCPULimit: inventory.Number(9000),
	})
	link(m, setup, inventory.PartOf, pod)

	assert.Empty(t, evaluate(NodeHealth{}, m, t0, node, nil).Findings)
}

// TestPodStorageNeedsEveryContainerLimited: a container without an
// ephemeral limit makes the summed limit meaningless for pod-wide use.
func TestPodStorageNeedsEveryContainerLimited(t *testing.T) {
	m, pod := podWithContainers(100)
	free := newID(kube.KindContainer, "default", "web/free")
	put(m, free, t0, nil)
	link(m, free, inventory.PartOf, pod)

	assert.Empty(t, evaluate(PodStorage{}, m, t0, pod, nil).Findings)
}
