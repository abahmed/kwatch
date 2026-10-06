package kube_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

type statsServer struct {
	mu    sync.Mutex
	calls map[string]int
}

func (s *statsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.calls[r.URL.Path]++
	n := s.calls[r.URL.Path]
	s.mu.Unlock()
	switch r.URL.Path {
	case "/n1/stats/summary":
		_, _ = fmt.Fprintf(w, summaryJSON, 10*n, 10*n)
	case "/n1/metrics/cadvisor":
		_, _ = fmt.Fprintf(w, "container_cpu_cfs_periods_total{id=\"/a\","+
			"namespace=\"ns\",pod=\"p\",container=\"c\"} %d\n"+
			"container_cpu_cfs_throttled_periods_total{id=\"/a\","+
			"namespace=\"ns\",pod=\"p\",container=\"c\"} %d\n"+
			"container_cpu_cfs_periods_total{id=\"/b\",namespace=\"\","+
			"pod=\"\",container=\"\"} 5\n", 100*n, 25*n)
	case "/n1/metrics":
		_, _ = fmt.Fprintf(w,
			"kubelet_runtime_operations_errors_total{op=\"x\"} %d\n", 5*n)
	default:
		http.NotFound(w, r)
	}
}

const summaryJSON = `{"node":{
"fs":{"capacityBytes":1000,"usedBytes":250,"inodes":100,"inodesFree":40},
"network":{"interfaces":[{"rxErrors":%d,"txErrors":%d}]},
"memory":{"psi":{"some":{"avg60":1.5}}},
"cpu":{"psi":{"some":{"avg60":2.5}}},
"io":{"psi":{"some":{"avg60":3.5}}}},
"pods":[{"podRef":{"name":"p","namespace":"ns"},
"containers":[{"name":"c","cpu":{"usageNanoCores":2000000},
"memory":{"workingSetBytes":4096}},{"name":"idle"}],
"ephemeral-storage":{"usedBytes":77},
"volume":[{"usedBytes":50,"capacityBytes":100,
"pvcRef":{"name":"data","namespace":"ns"}},
{"usedBytes":1,"capacityBytes":0,
"pvcRef":{"name":"empty","namespace":"ns"}},{"usedBytes":3}]},
{"podRef":{}}]}`

func TestStatsPollerRecordsUsageAndRates(t *testing.T) {
	kubelet := handlerKubelet{&statsServer{calls: map[string]int{}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batches := make(chan []inventory.Observation, 4)
	now := fixedTime()
	poller := kube.NewStatsPoller(kube.StatsConfig{
		Kubelet: kubelet, Interval: time.Millisecond,
		Now: func() time.Time { now = now.Add(time.Second); return now },
		Submit: func(_ context.Context, f ...inventory.Observation) {
			batches <- f
		},
		Nodes: func() []inventory.EntityID {
			return []inventory.EntityID{
				inventory.CoreID(kube.KindNode, "", "n1"),
			}
		},
	})
	done := make(chan struct{})
	go func() { poller.Run(ctx); close(done) }()
	first, second := <-batches, <-batches
	cancel()
	<-done

	attr := func(observations []inventory.Observation, id inventory.EntityID,
		name string) (float64, bool) {
		for _, f := range observations {
			if f.Entity == id {
				if v, ok := f.Attributes[name]; ok {
					n, _ := v.AsNumber()
					return n, true
				}
			}
		}
		return 0, false
	}
	node := inventory.CoreID(kube.KindNode, "", "n1")
	v, ok := attr(first, node, kube.AttrFSUsedPct)
	require.True(t, ok)
	assert.InDelta(t, 25, v, 0.001)
	v, _ = attr(first, node, kube.AttrInodesUsedPct)
	assert.InDelta(t, 60, v, 0.001)
	v, _ = attr(first, node, kube.AttrMemoryPSI)
	assert.InDelta(t, 1.5, v, 0.001)
	_, ok = attr(first, node, kube.AttrNetErrorRate)
	assert.False(t, ok, "first sample has no rate")
	ctr := kube.ContainerID("ns", "p", "c")
	v, _ = attr(first, ctr, kube.AttrCPUUsageMilli)
	assert.InDelta(t, 2, v, 0.001)
	v, _ = attr(first, ctr, kube.AttrMemoryWorking)
	assert.InDelta(t, 4096, v, 0.001)
	v, _ = attr(first, ctr, kube.AttrMemoryPeak24h)
	assert.InDelta(t, 4096, v, 0.001, "the history starts with the reading")
	pod := inventory.CoreID(kube.KindPod, "ns", "p")
	v, _ = attr(first, pod, kube.AttrEphemeralUsed)
	assert.InDelta(t, 77, v, 0.001)
	claim := inventory.CoreID(kube.KindPVC, "ns", "data")
	v, _ = attr(first, claim, kube.AttrVolumeUsedPct)
	assert.InDelta(t, 50, v, 0.001)
	empty := inventory.CoreID(kube.KindPVC, "ns", "empty")
	_, ok = attr(first, empty, kube.AttrVolumeUsedPct)
	assert.False(t, ok)

	v, ok = attr(second, node, kube.AttrNetErrorRate)
	require.True(t, ok)
	assert.InDelta(t, 20, v, 0.001)
	v, ok = attr(second, ctr, kube.AttrThrottledPct)
	require.True(t, ok)
	assert.InDelta(t, 25, v, 0.001)
	v, ok = attr(second, node, kube.AttrRuntimeErrRate)
	require.True(t, ok)
	assert.InDelta(t, 5, v, 0.001)
}

func TestStatsPollerSkipsUnavailableNode(t *testing.T) {
	kubelet := handlerKubelet{http.NotFoundHandler()}
	ctx, cancel := context.WithCancel(context.Background())
	polled := make(chan struct{})
	poller := kube.NewStatsPoller(kube.StatsConfig{
		Kubelet: kubelet, Now: fixedTime,
		Submit: func(_ context.Context, obs ...inventory.Observation) {
			// Only the failure count may be submitted for a dead node.
			for _, o := range obs {
				if _, ok := o.Attributes[kube.AttrKubeletFailures]; !ok {
					t.Error("unexpected submit")
				}
			}
		},
		Nodes: func() []inventory.EntityID {
			defer func() {
				select {
				case <-polled:
				default:
					close(polled)
				}
			}()
			return []inventory.EntityID{
				inventory.CoreID(kube.KindNode, "", "n1"),
			}
		},
	})
	done := make(chan struct{})
	go func() { poller.Run(ctx); close(done) }()
	<-polled
	cancel()
	<-done
}

// Kubelet stats can still report a pod the informers already deleted. A
// model configured with the stats sources must not bring it back.
func TestStatsObservationsDoNotResurrectDeletedEntities(t *testing.T) {
	kubelet := handlerKubelet{&statsServer{calls: map[string]int{}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batches := make(chan []inventory.Observation, 1)
	node := inventory.CoreID(kube.KindNode, "", "n1")
	poller := kube.NewStatsPoller(kube.StatsConfig{
		Kubelet: kubelet, Now: fixedTime,
		Submit: func(_ context.Context, f ...inventory.Observation) {
			select {
			case batches <- f:
			default:
			}
		},
		Nodes: func() []inventory.EntityID {
			return []inventory.EntityID{node}
		},
	})
	done := make(chan struct{})
	go func() { poller.Run(ctx); close(done) }()
	observations := <-batches
	cancel()
	<-done

	model := inventory.NewModel(inventory.Options{
		EnrichmentSources: kube.EnrichmentSources(),
	})
	pod := inventory.CoreID(kube.KindPod, "ns", "p")
	for _, f := range []inventory.Observation{
		{Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: node},
		{Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: pod},
		{Kind: inventory.Gone, Source: kube.ObservationSource,
			At: fixedTime(), Entity: pod},
	} {
		_, err := model.Apply(f)
		require.NoError(t, err)
	}
	for _, f := range observations {
		_, err := model.Apply(f)
		require.NoError(t, err)
	}

	assert.False(t, model.Exists(pod), "deleted pod resurrected")
	assert.False(t, model.Exists(kube.ContainerID("ns", "p", "c")))
	assert.False(t, model.Exists(
		inventory.CoreID(kube.KindPVC, "ns", "data")))
	entity, ok := model.Entity(node)
	require.True(t, ok)
	assert.Contains(t, entity.Attributes, kube.AttrFSUsedPct)
}
