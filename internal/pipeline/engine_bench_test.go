package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Benchmark cluster: 5,000 pods in 1,000 deployments on 50 nodes. 500
// deployments have 2 crashing pods each (1,000 failing pods); the other
// 500 have 8 healthy pods each.
const (
	benchNodes          = 50
	benchFailingWorkers = 500
	benchHealthyWorkers = 500
	benchFailingPods    = 2
	benchHealthyPods    = 8
)

// benchCluster is a settled engine over the benchmark cluster and the
// update observations of its failing pods, in two alternating versions.
type benchCluster struct {
	h       *harness
	updates [2][]inventory.Observation
}

func newBenchCluster(b testing.TB) *benchCluster {
	b.Helper()
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h := newHarness(b, start)
	for i := range benchNodes {
		h.add(kube.NodeSchema{}, node(fmt.Sprintf("n%d", i), time.Time{}))
	}
	c := &benchCluster{h: h}
	pods := kube.NewTranslator(kube.PodSchema{})
	for w := range benchFailingWorkers + benchHealthyWorkers {
		name := fmt.Sprintf("app%d", w)
		d, rs := deployment(name)
		h.add(kube.DeploymentSchema(), d)
		h.add(kube.ReplicaSetSchema(), rs)
		failing := w < benchFailingWorkers
		count := benchHealthyPods
		if failing {
			count = benchFailingPods
		}
		for i := range count {
			podName := fmt.Sprintf("%s-%d", name, i)
			nodeName := fmt.Sprintf("n%d", (w+i)%benchNodes)
			if !failing {
				h.add(kube.PodSchema{},
					pod(podName, rs.Name, nodeName, true, start))
				continue
			}
			p := crashingPod(podName, rs.Name, nodeName, start)
			h.add(kube.PodSchema{}, p)
			next := p.DeepCopy()
			next.Status.ContainerStatuses[0].RestartCount++
			c.updates[0] = append(c.updates[0],
				pods.Updated(p, next, start)...)
			c.updates[1] = append(c.updates[1],
				pods.Updated(next, p, start)...)
		}
		// Drain as the loop would, so the initial list never fills the
		// pending queue.
		h.engine.step(context.Background(), h.now, h.checks)
	}
	h.run(start.Add(10*time.Minute), 10*time.Second)
	if open := len(h.engine.Incidents()); open == 0 {
		b.Fatal("benchmark cluster raised no incidents")
	}
	return c
}

// One loop iteration in which every failing pod changed.
func BenchmarkEngineStepFailingPodsChanged(b *testing.B) {
	c := newBenchCluster(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		c.h.engine.Submit(ctx, c.updates[i%2]...)
		c.h.now = c.h.now.Add(time.Second)
		c.h.engine.step(ctx, c.h.now, c.h.checks)
	}
	b.ReportMetric(float64(len(c.h.engine.Incidents())), "incidents")
}

// One loop iteration with nothing new: only lifecycle deadlines.
func BenchmarkEngineStepIdle(b *testing.B) {
	c := newBenchCluster(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		c.h.now = c.h.now.Add(time.Second)
		c.h.engine.step(ctx, c.h.now, c.h.checks)
	}
}

// The loop's share of persistence: building the snapshot it hands to the
// storage writer.
func BenchmarkEngineSnapshot(b *testing.B) {
	c := newBenchCluster(b)
	c.h.engine.storage.reconciled = true
	c.h.engine.storage.incidents = newStoreWriter(&memStore{}, wallTimer,
		&c.h.engine.stats)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		c.h.engine.save()
	}
}

// The loop's persistence cost per decided iteration: the gate builds at
// most one snapshot per batch interval however many iterations decide.
func BenchmarkEngineLoopPersist(b *testing.B) {
	c := newBenchCluster(b)
	e := c.h.engine
	e.storage.reconciled = true
	e.storage.incidents = newStoreWriter(&memStore{}, wallTimer, &e.stats)
	gate := &saveGate{}
	now := c.h.now
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		now = now.Add(100 * time.Millisecond)
		e.persist(gate, now, true)
	}
}
