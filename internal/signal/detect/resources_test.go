package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func containerWith(attrs map[string]knowledge.Value,
) (*knowledge.Model, knowledge.EntityID) {
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
		severity signal.Severity
	}{
		{"quiet below 90", 89, false, 0},
		{"warning at 90", 90, true, signal.Warning},
		{"critical at 95", 95, true, signal.Critical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, id := containerWith(map[string]knowledge.Value{
				kube.AttrMemoryWorking: knowledge.Number(tt.used),
				kube.AttrMemoryLimit:   knowledge.Number(100),
			})
			got := evaluate(ContainerResources{}, m, t0, id, nil).Signals
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, constant.ReasonContainerMemoryHigh,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestContainerResourcesCPUSustained(t *testing.T) {
	m, id := containerWith(map[string]knowledge.Value{
		kube.AttrCPUUsageMilli: knowledge.Number(95),
		kube.AttrCPULimit:      knowledge.Number(100),
	})
	early := evaluate(ContainerResources{}, m, t0.Add(4*time.Minute), id,
		nil)
	assert.Empty(t, early.Signals)
	assert.Equal(t, 6*time.Minute, early.RecheckAfter)

	late := evaluate(ContainerResources{}, m, t0.Add(10*time.Minute), id,
		nil)
	require.Len(t, late.Signals, 1)
	assert.Equal(t, constant.ReasonContainerCPUHigh, late.Signals[0].Reason)
	assert.Zero(t, late.RecheckAfter)
}

func TestContainerResourcesThrottled(t *testing.T) {
	tests := []struct {
		name     string
		pct      float64
		fires    bool
		severity signal.Severity
	}{
		{"quiet below 50", 49, false, 0},
		{"warning at 50", 50, true, signal.Warning},
		{"critical at 75", 75, true, signal.Critical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, id := containerWith(map[string]knowledge.Value{
				kube.AttrThrottledPct: knowledge.Number(tt.pct),
			})
			now := t0.Add(11 * time.Minute)
			got := evaluate(ContainerResources{}, m, now, id, nil).Signals
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, constant.ReasonContainerCPUThrottled,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
		})
	}
}

func TestContainerResourcesThrottledPendingRecheck(t *testing.T) {
	m, id := containerWith(map[string]knowledge.Value{
		kube.AttrThrottledPct: knowledge.Number(80),
	})
	got := evaluate(ContainerResources{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, got.Signals)
	assert.Equal(t, 9*time.Minute, got.RecheckAfter)
	assert.Equal(t, "container-resources", ContainerResources{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindContainer},
		ContainerResources{}.Kinds())
}

func podWithContainers(limits ...float64,
) (*knowledge.Model, knowledge.EntityID) {
	m := newTestModel()
	pod := newID(kube.KindPod, "default", "web")
	put(m, pod, t0, map[string]knowledge.Value{
		kube.AttrEphemeralUsed: knowledge.Number(90),
	})
	for i, limit := range limits {
		c := newID(kube.KindContainer, "default",
			"web/c"+string(rune('a'+i)))
		put(m, c, t0, map[string]knowledge.Value{
			kube.AttrEphemeralLimit: knowledge.Number(limit),
		})
		link(m, c, knowledge.PartOf, pod)
	}
	return m, pod
}

func TestPodStorageNearLimit(t *testing.T) {
	m, pod := podWithContainers(60, 40)
	got := evaluate(PodStorage{}, m, t0, pod, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonContainerEphemeralStorageHigh,
		got[0].Reason)
	assert.Equal(t, signal.Warning, got[0].Severity)
	assert.NotEmpty(t, got[0].Summary)
}

func TestPodStorageCritical(t *testing.T) {
	m, pod := podWithContainers(94)
	got := evaluate(PodStorage{}, m, t0, pod, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, signal.Critical, got[0].Severity)
}

func TestPodStorageQuiet(t *testing.T) {
	m, pod := podWithContainers(101)
	assert.Empty(t, evaluate(PodStorage{}, m, t0, pod, nil).Signals)

	m, pod = podWithContainers()
	assert.Empty(t, evaluate(PodStorage{}, m, t0, pod, nil).Signals,
		"no limit declared")

	m = newTestModel()
	bare := newID(kube.KindPod, "default", "bare")
	put(m, bare, t0, nil)
	assert.Empty(t, evaluate(PodStorage{}, m, t0, bare, nil).Signals,
		"no usage reported")
	assert.Equal(t, "pod-storage", PodStorage{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindPod}, PodStorage{}.Kinds())
}

func TestNodeHealthErrorRates(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrNetErrorRate:   knowledge.Number(1),
		kube.AttrRuntimeErrRate: knowledge.Number(10),
	})
	got := evaluate(NodeHealth{}, m, t0, id, nil).Signals
	require.Len(t, got, 2)
	assert.Equal(t, constant.ReasonNodeNetworkErrors, got[0].Reason)
	assert.Equal(t, signal.Warning, got[0].Severity)
	assert.Equal(t, constant.ReasonNodeRuntimeErrors, got[1].Reason)
	assert.Equal(t, signal.Critical, got[1].Severity)
	assert.NotEmpty(t, got[0].Summary)
}

func TestNodeHealthQuietBelowOne(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrNetErrorRate: knowledge.Number(0.9),
	})
	assert.Empty(t, evaluate(NodeHealth{}, m, t0, id, nil).Signals)
	assert.Equal(t, "node-health", NodeHealth{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindNode}, NodeHealth{}.Kinds())
}

func overcommittedNode(cpuLimit, memLimit float64,
) (*knowledge.Model, knowledge.EntityID) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]knowledge.Value{
		kube.AttrCPUAllocatable:    knowledge.Number(1000),
		kube.AttrMemoryAllocatable: knowledge.Number(1000),
	})
	pod := newID(kube.KindPod, "default", "web")
	put(m, pod, t0, nil)
	link(m, pod, knowledge.RunsOn, node)
	c := newID(kube.KindContainer, "default", "web/app")
	put(m, c, t0, map[string]knowledge.Value{
		kube.AttrCPULimit:    knowledge.Number(cpuLimit),
		kube.AttrMemoryLimit: knowledge.Number(memLimit),
	})
	link(m, c, knowledge.PartOf, pod)
	return m, node
}

func TestNodeHealthOvercommit(t *testing.T) {
	tests := []struct {
		name     string
		cpu, mem float64
		reason   string
		severity signal.Severity
		fires    bool
	}{
		{"quiet under 2x", 1999, 100, "", 0, false},
		{"high at 2x", 2000, 100, constant.ReasonNodeResourceHigh,
			signal.Warning, true},
		{"critical at 4x memory", 100, 4000,
			constant.ReasonNodeResourceCritical, signal.Critical, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, node := overcommittedNode(tt.cpu, tt.mem)
			got := evaluate(NodeHealth{}, m, t0, node, nil).Signals
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
	assert.Empty(t, evaluate(NodeHealth{}, m, t0, id, nil).Signals)
}
