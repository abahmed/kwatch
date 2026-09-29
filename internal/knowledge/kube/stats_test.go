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

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
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
	case "/api/v1/nodes/n1/proxy/stats/summary":
		_, _ = fmt.Fprintf(w, summaryJSON, 10*n, 10*n)
	case "/api/v1/nodes/n1/proxy/metrics/cadvisor":
		_, _ = fmt.Fprintf(w, "container_cpu_cfs_periods_total{id=\"/a\","+
			"namespace=\"ns\",pod=\"p\",container=\"c\"} %d\n"+
			"container_cpu_cfs_throttled_periods_total{id=\"/a\","+
			"namespace=\"ns\",pod=\"p\",container=\"c\"} %d\n"+
			"container_cpu_cfs_periods_total{id=\"/b\",namespace=\"\","+
			"pod=\"\",container=\"\"} 5\n", 100*n, 25*n)
	case "/api/v1/nodes/n1/proxy/metrics":
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
	client := restClient(t, &statsServer{calls: map[string]int{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batches := make(chan []knowledge.Fact, 4)
	now := fixedTime()
	poller := kube.NewStatsPoller(kube.StatsConfig{
		Client: client, Interval: time.Millisecond,
		Now: func() time.Time { now = now.Add(time.Second); return now },
		Submit: func(_ context.Context, f ...knowledge.Fact) {
			batches <- f
		},
		Nodes: func() []knowledge.EntityID {
			return []knowledge.EntityID{
				knowledge.NewEntityID(kube.KindNode, "", "n1"),
			}
		},
	})
	done := make(chan struct{})
	go func() { poller.Run(ctx); close(done) }()
	first, second := <-batches, <-batches
	cancel()
	<-done

	attr := func(facts []knowledge.Fact, id knowledge.EntityID,
		name string) (float64, bool) {
		for _, f := range facts {
			if f.Entity == id {
				if v, ok := f.Attributes[name]; ok {
					n, _ := v.AsNumber()
					return n, true
				}
			}
		}
		return 0, false
	}
	node := knowledge.NewEntityID(kube.KindNode, "", "n1")
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
	pod := knowledge.NewEntityID(kube.KindPod, "ns", "p")
	v, _ = attr(first, pod, kube.AttrEphemeralUsed)
	assert.InDelta(t, 77, v, 0.001)
	claim := knowledge.NewEntityID(kube.KindPVC, "ns", "data")
	v, _ = attr(first, claim, kube.AttrVolumeUsedPct)
	assert.InDelta(t, 50, v, 0.001)
	empty := knowledge.NewEntityID(kube.KindPVC, "ns", "empty")
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
	client := restClient(t, http.NotFoundHandler())
	ctx, cancel := context.WithCancel(context.Background())
	polled := make(chan struct{})
	poller := kube.NewStatsPoller(kube.StatsConfig{
		Client: client, Now: fixedTime,
		Submit: func(context.Context, ...knowledge.Fact) {
			t.Error("unexpected submit")
		},
		Nodes: func() []knowledge.EntityID {
			defer func() {
				select {
				case <-polled:
				default:
					close(polled)
				}
			}()
			return []knowledge.EntityID{
				knowledge.NewEntityID(kube.KindNode, "", "n1"),
			}
		},
	})
	done := make(chan struct{})
	go func() { poller.Run(ctx); close(done) }()
	<-polled
	cancel()
	<-done
}
