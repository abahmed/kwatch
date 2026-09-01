package integration

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/correlation"
)

func TestBoundedStateUnderLoad(t *testing.T) {
	rec := &recordingAlertManager{}
	eng := correlation.NewEngine(defaultConfig(rec))

	// 10 distinct owners × 100 events each = 1000 total events
	distinctOwners := 10
	eventsPerOwner := 100

	cs := makeContainerState(1, "OOMKill", 137)

	for o := 0; o < distinctOwners; o++ {
		owner := fmt.Sprintf("dep-%d", o)
		for i := 0; i < eventsPerOwner; i++ {
			podName := fmt.Sprintf("pod-%d", i)
			ev := makeEvent("pod", podName, "load-ns", "OOMKill", "main", "")
			eng.Process(ev, owner, cs)
		}
	}

	// The engine should hold exactly distinctOwners entries (one per owner)
	// in its active state. We can't access state directly, but we can verify
	// indirectly: resolving each triggers a single notification.
	for o := 0; o < distinctOwners; o++ {
		key := correlation.BuildKey(
			"load-ns",
			fmt.Sprintf("dep-%d", o),
			"OOMKill",
			"",
		)
		eng.MarkResolved(key)
	}

	// DistinctOwners creates + distinctOwners resolves = 2*distinctOwners in
	// best case; storm playbook may add extra. Sanity: << 1000.
	if rec.Len() > 100 {
		t.Fatalf("expected bounded notifications <= 100, got %d", rec.Len())
	}
}

// BenchmarkProcess measures allocation and throughput of engine.Process
// under bulk-load conditions.
func BenchmarkProcess(b *testing.B) {
	rec := &recordingAlertManager{}
	eng := correlation.NewEngine(defaultConfig(rec))

	owner := "dep-bench"
	cs := makeContainerState(1, "CrashLoopBackOff", 137)

	ev := makeEvent("pod", "pod", "bench-ns", "CrashLoopBackOff", "main", "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eng.Process(ev, owner, cs)
	}
}
