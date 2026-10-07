package scenarios

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/replay"
)

// The soak test replays a simulated day through the engine and measures
// the live heap each simulated hour. Memory must plateau: after the
// warm-up the heap may not keep climbing. The model is pruned every ten
// simulated minutes with the application's retention, as the
// application does.
const (
	soakLength    = 24 * time.Hour
	soakWarmup    = 6 * time.Hour
	soakPrune     = 10 * time.Minute
	soakRetention = 2 * time.Hour
	// soakMaxGrowthMiBPerHour is how much the heap may still climb per
	// hour after the warm-up; it is far below the cost of a real leak.
	soakMaxGrowthMiBPerHour = 0.5
)

// liveHeap is the heap in use after a full collection, in MiB.
func liveHeap() float64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return float64(stats.HeapAlloc) / (1 << 20)
}

func TestSoakHeapPlateaus(t *testing.T) {
	if testing.Short() {
		t.Skip("soak replay runs a simulated day")
	}
	if raceEnabled {
		t.Skip("the heap is measured without -race: make verify-latency")
	}
	log := soakLog(soakLength)
	deps := newDependencies()
	model := deps.Model
	var hours []float64
	start := log.Start
	opts := replay.Options{
		Tail: time.Hour, Every: soakPrune,
		Tick: func(at time.Time) {
			model.Prune(at.Add(-soakRetention))
			if at.Sub(start)%time.Hour != 0 {
				return
			}
			heap := liveHeap()
			hours = append(hours, heap)
			stats := model.Stats()
			t.Logf("hour %2d: heap %6.1f MiB entities %d tombstones %d "+
				"changes %d relations %d", len(hours), heap, stats.Entities,
				stats.Tombstones, stats.Changes, stats.Relations)
			switch at.Sub(start) {
			case soakWarmup:
				soakProfile(t, "warm")
			case soakLength:
				soakProfile(t, "end")
			}
		},
	}
	if _, err := replay.Run(t.Context(), log, deps, opts); err != nil {
		t.Fatal(err)
	}
	if len(hours) < int(soakLength/time.Hour) {
		t.Fatalf("only %d hourly samples", len(hours))
	}
	from := int(soakWarmup / time.Hour)
	last := int(soakLength/time.Hour) - 1
	rate := (hours[last] - hours[from]) / float64(last-from)
	t.Logf("heap growth after warm-up: %.2f MiB/hour", rate)
	if rate > soakMaxGrowthMiBPerHour {
		t.Errorf("heap grows %.2f MiB per simulated hour after warm-up; "+
			"want at most %.1f", rate, soakMaxGrowthMiBPerHour)
	}
}

// soakProfile writes a heap profile when KWATCH_SOAK_PROFILE names a
// directory, for `go tool pprof -top -base warm.pprof end.pprof`.
func soakProfile(t *testing.T, name string) {
	dir := os.Getenv("KWATCH_SOAK_PROFILE")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".pprof"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}
