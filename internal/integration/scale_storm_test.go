package integration

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// TestEngineStormScale replays a large-cluster storm: 5,000 pods in 500
// workloads on 50 nodes fail at once. It reports throughput, retained heap,
// and messages per failing workload, and bounds processing time so a
// regression to per-event scans of all incidents fails loudly. Stricter
// message budgets belong to the redesigned core's scorecard.
func TestEngineStormScale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test")
	}
	const (
		workloads   = 500
		podsPerLoad = 10
		nodes       = 50
	)
	rec := &recordingDelivery{}
	eng := newTestIncidentEngine(defaultConfig(rec))
	cs := makeContainerState(3, "CrashLoopBackOff", 1)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	for w := 0; w < workloads; w++ {
		owner := fmt.Sprintf("app-%d", w)
		namespace := fmt.Sprintf("ns-%d", w%20)
		for p := 0; p < podsPerLoad; p++ {
			ev := makeEvent(
				"pod", fmt.Sprintf("%s-%d", owner, p), namespace,
				"CrashLoopBackOff", "main",
				fmt.Sprintf("node-%d", (w*podsPerLoad+p)%nodes),
			)
			eng.Process(ownedBy(ev, owner, cs))
		}
	}
	elapsed := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)

	events := workloads * podsPerLoad
	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("events=%d elapsed=%s per-event=%s retained-heap=%dKiB "+
		"messages=%d messages-per-workload=%.2f",
		events, elapsed, elapsed/time.Duration(events), retained/1024,
		rec.Len(), float64(rec.Len())/workloads)
	actions := map[string]int{}
	for i := 0; i < rec.Len(); i++ {
		_, action := rec.Get(i)
		actions[action.String()]++
	}
	t.Logf("actions=%v", actions)
	if elapsed > 30*time.Second {
		t.Fatalf("storm processing took %s, want < 30s", elapsed)
	}
	// Today every extra failing pod re-announces its workload as an
	// update (4,500 here); the redesign's budget is a few messages for the
	// whole storm. Until then only the create count is enforced.
	if actions["create"] > workloads {
		t.Fatalf("storm created %d incidents for %d workloads",
			actions["create"], workloads)
	}
}
