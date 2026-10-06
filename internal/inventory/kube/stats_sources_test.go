package kube

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// fakeKubelet serves a body per "node/path". A missing body is an error.
type fakeKubelet struct {
	mu     sync.Mutex
	bodies map[string]string
	fail   map[string]error
}

func (k *fakeKubelet) set(key, body string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.bodies == nil {
		k.bodies = map[string]string{}
	}
	k.bodies[key] = body
}

func (k *fakeKubelet) remove(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.bodies, key)
}

func (k *fakeKubelet) failWith(node string, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.fail == nil {
		k.fail = map[string]error{}
	}
	k.fail[node] = err
}

func (k *fakeKubelet) Open(
	_ context.Context, node, path string,
) (io.ReadCloser, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err := k.fail[node]; err != nil {
		return nil, err
	}
	body, ok := k.bodies[node+"/"+path]
	if !ok {
		return nil, errors.New("not served")
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

// sourcesHarness runs the poller into a model with the given nodes.
type sourcesHarness struct {
	t       *testing.T
	kubelet *fakeKubelet
	poller  *StatsPoller
	model   *inventory.Model
	nodes   []string
	now     time.Time
}

func newSourcesHarness(t *testing.T, nodes ...string) *sourcesHarness {
	h := &sourcesHarness{
		t: t, kubelet: &fakeKubelet{}, nodes: nodes, now: fixedNow(),
	}
	h.model = inventory.NewModel(inventory.Options{
		EnrichmentSources: EnrichmentSources(),
	})
	h.poller = NewStatsPoller(StatsConfig{
		Kubelet: h.kubelet, Now: func() time.Time { return h.now },
		Nodes: func() []inventory.EntityID { return statsNodes(h.nodes...)() },
		Containers: func() []inventory.EntityID {
			return h.model.Entities(KindContainer)
		},
		Submit: func(_ context.Context, obs ...inventory.Observation) {
			for _, o := range obs {
				_, err := h.model.Apply(o)
				require.NoError(t, err)
			}
		},
	})
	return h
}

// observe makes entities present, as the informers do.
func (h *sourcesHarness) observe(ids ...inventory.EntityID) {
	for _, id := range ids {
		_, err := h.model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: ObservationSource,
			At: h.now, Entity: id,
		})
		require.NoError(h.t, err)
	}
}

func (h *sourcesHarness) poll() {
	h.now = h.now.Add(time.Minute)
	h.poller.poll(context.Background())
}

func (h *sourcesHarness) has(id inventory.EntityID, attr string) bool {
	entity, ok := h.model.Entity(id)
	require.True(h.t, ok)
	_, has := entity.Attributes[attr]
	return has
}

const nodeSummary = `{"node":{"fs":{"usedBytes":90,"capacityBytes":100}}}`

func TestFailedKubeletReadsKeepTheNodeReadings(t *testing.T) {
	h := newSourcesHarness(t, "a")
	node := inventory.CoreID(KindNode, "", "a")
	h.observe(node)
	h.kubelet.set("a/stats/summary", nodeSummary)
	h.poll()
	require.True(t, h.has(node, AttrFSUsedPct))

	h.kubelet.failWith("a", errors.New("i/o timeout"))
	h.poll()
	assert.True(t, h.has(node, AttrKubeletFailures), "the miss is counted")
	assert.True(t, h.has(node, AttrFSUsedPct),
		"counting a miss must not wipe the last reading")

	h.kubelet.failWith("a", nil)
	h.poll()
	assert.True(t, h.has(node, AttrFSUsedPct),
		"a recovered poll must not wipe its own reading")
}

const podWithMemory = `{"pods":[{"podRef":{"name":"p","namespace":"ns"},
"containers":[{"name":"c","startTime":"2026-01-01T00:00:00Z",
"memory":{"workingSetBytes":600000000}}]}]}`

const podWithoutContainer = `{"pods":[{"podRef":{"name":"p",
"namespace":"ns"},"containers":[]}]}`

func TestContainerRSSIsRecordedWhenTheKubeletReportsIt(t *testing.T) {
	h := newSourcesHarness(t, "a")
	container := ContainerID("ns", "p", "c")
	h.observe(container)
	h.kubelet.set("a/stats/summary", podWithMemory)
	h.poll()
	assert.False(t, h.has(container, AttrMemoryRSS))

	h.kubelet.set("a/stats/summary", strings.Replace(podWithMemory,
		`"workingSetBytes":600000000`,
		`"workingSetBytes":600000000,"rssBytes":400000000`, 1))
	h.poll()
	assert.True(t, h.has(container, AttrMemoryRSS))
}

func TestMemoryHistoryOutlivesTheContainerInTheSummary(t *testing.T) {
	h := newSourcesHarness(t, "a")
	container := ContainerID("ns", "p", "c")
	h.observe(container)
	h.kubelet.set("a/stats/summary", podWithMemory)
	h.poll()
	require.True(t, h.has(container, AttrMemoryPeak24h))

	// A crashing container is missing from the summary.
	h.kubelet.set("a/stats/summary", podWithoutContainer)
	h.poll()
	assert.False(t, h.has(container, AttrMemoryWorking),
		"the live reading is gone")
	assert.True(t, h.has(container, AttrMemoryPeak24h),
		"the history is what explains the OOM kill")

	// Even when the kubelet cannot be read for a while.
	h.kubelet.failWith("a", errors.New("i/o timeout"))
	for range statsFailuresBeforeClear {
		h.poll()
	}
	assert.True(t, h.has(container, AttrMemoryPeak24h))
}

func TestMemoryHistoryIsRemovedAfterItsWindow(t *testing.T) {
	h := newSourcesHarness(t, "a")
	container := ContainerID("ns", "p", "c")
	h.observe(container)
	h.kubelet.set("a/stats/summary", podWithMemory)
	h.poll()
	h.kubelet.set("a/stats/summary", podWithoutContainer)
	h.now = h.now.Add((memoryWindowHours + 1) * time.Hour)
	h.poll()
	assert.False(t, h.has(container, AttrMemoryPeak24h))
}

func TestMemoryHistoryIsRemovedWhenTheContainerLeavesTheModel(t *testing.T) {
	h := newSourcesHarness(t, "a")
	container := ContainerID("ns", "p", "c")
	h.observe(container)
	h.kubelet.set("a/stats/summary", podWithMemory)
	h.poll()
	require.True(t, h.has(container, AttrMemoryPeak24h))

	_, err := h.model.Apply(inventory.Observation{
		Kind: inventory.Gone, Source: ObservationSource, At: h.now,
		Entity: container,
	})
	require.NoError(t, err)
	h.kubelet.set("a/stats/summary", podWithoutContainer)
	h.poll()
	assert.Empty(t, h.poller.memory.containers,
		"a deleted container's history is no longer republished")

	h.observe(container)
	assert.False(t, h.has(container, AttrMemoryPeak24h),
		"a new pod of the same name does not inherit it")
}

func metricsBody(errors int) string {
	return "kubelet_runtime_operations_errors_total{operation_type=\"x\"} " +
		strings.Repeat("1", errors) + "\n"
}

func TestOptionalMetricsGetTheSameGraceAsTheSummary(t *testing.T) {
	h := newSourcesHarness(t, "a")
	node := inventory.CoreID(KindNode, "", "a")
	h.observe(node)
	h.kubelet.set("a/stats/summary", nodeSummary)
	h.kubelet.set("a/metrics", metricsBody(1))
	h.poll()
	h.kubelet.set("a/metrics", metricsBody(2))
	h.poll()
	require.True(t, h.has(node, AttrRuntimeErrRate))

	h.kubelet.remove("a/metrics")
	for i := 1; i < statsFailuresBeforeClear; i++ {
		h.poll()
		assert.True(t, h.has(node, AttrRuntimeErrRate),
			"miss %d keeps the last reading", i)
	}
	h.poll()
	assert.False(t, h.has(node, AttrRuntimeErrRate))
}

func claimObservation(source string) inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Observed, Source: source,
		Entity: inventory.CoreID(KindPVC, "ns", "data"),
		Attributes: map[string]inventory.Value{
			AttrVolumeUsedPct: inventory.Number(50),
		},
	}
}

func TestOnlyTheLatestNodeClearsAMovedEntity(t *testing.T) {
	for _, order := range [][2]string{{"a", "b"}, {"b", "a"}} {
		log := newPublishLog()
		now := fixedNow()
		log.answered("a", []inventory.Observation{
			claimObservation(StatsSource)}, now)
		// The claim moved: b reports it, a no longer does.
		var cleared []inventory.Observation
		for _, node := range order {
			var reported []inventory.Observation
			if node == "b" {
				reported = []inventory.Observation{
					claimObservation(StatsSource)}
			}
			cleared = append(cleared, log.answered(node, reported, now)...)
		}
		if order[0] == "b" {
			assert.Empty(t, cleared, "a must not wipe what b reports")
		}
	}
}

func TestNodesThatLeftTheClusterAreForgotten(t *testing.T) {
	h := newSourcesHarness(t, "a")
	node := inventory.CoreID(KindNode, "", "a")
	h.observe(node)
	h.kubelet.set("a/stats/summary", nodeSummary)
	h.poll()
	require.True(t, h.has(node, AttrFSUsedPct))

	h.nodes = nil
	h.poll()
	assert.False(t, h.has(node, AttrFSUsedPct))
	assert.Empty(t, h.poller.published.nodes)
	assert.Empty(t, h.poller.published.owners)
}

func TestInodesFreeAboveTotalIsNotReported(t *testing.T) {
	p := NewStatsPoller(StatsConfig{})
	var s statsSummary
	s.Node.FS.Inodes = 100
	s.Node.FS.InodesFree = 150
	obs := p.observations(inventory.CoreID(KindNode, "", "a"), s, fixedNow())
	assert.NotContains(t, obs[0].Attributes, AttrInodesUsedPct)

	s.Node.FS.InodesFree = 40
	obs = p.observations(inventory.CoreID(KindNode, "", "a"), s, fixedNow())
	assert.InDelta(t, 60.0, attrNumber(obs[0].Attributes, AttrInodesUsedPct),
		0.001)
}
