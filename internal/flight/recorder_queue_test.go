package flight

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

// numbered returns n pod observations whose restart counts are from,
// from+1, ...; the sanitizer keeps numbers, so a test can read the order.
func numbered(from, n int) []inventory.Observation {
	out := make([]inventory.Observation, n)
	for i := range out {
		out[i] = podObserved(fmt.Sprintf("p-%d", from+i), float64(from+i))
	}
	return out
}

// fullRecorder returns a recorder whose queue is exactly full.
func fullRecorder(t *testing.T) *Recorder {
	t.Helper()
	rec, err := NewRecorder(Ring{}, "salt", newClock().Now)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; rec.Pending() < maxPending; i++ {
		rec.record(numbered(i, 1))
	}
	return rec
}

func TestRecorderFullQueueCostStaysFlat(t *testing.T) {
	rec := fullRecorder(t)
	const calls = 400
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < calls; i++ {
		rec.record(numbered(i, 5))
	}
	runtime.ReadMemStats(&after)
	perCall := (after.TotalAlloc - before.TotalAlloc) / calls
	// Copying the whole queue on every call costs about maxPending
	// entries (over a megabyte); the amortized cost must stay far below.
	if perCall > 64<<10 {
		t.Errorf("a full queue allocates %d bytes per call, want < 64 KiB",
			perCall)
	}
}

func TestRecorderFullQueueKeepsMemoryBounded(t *testing.T) {
	rec := fullRecorder(t)
	for i := 0; i < 4*maxPending; i++ {
		rec.record(numbered(i, 1))
		if rec.Pending() > maxPending {
			t.Fatalf("pending = %d, bound is %d", rec.Pending(), maxPending)
		}
		if got := cap(rec.pending); got > 2*maxPending {
			t.Fatalf("backing array holds %d slots, bound is %d",
				got, 2*maxPending)
		}
	}
	if rec.Pending() != maxPending {
		t.Errorf("pending = %d, want a full queue of %d",
			rec.Pending(), maxPending)
	}
}

func TestRecorderFullQueueKeepsTheNewestEntriesInOrder(t *testing.T) {
	rec := fullRecorder(t)
	const extra = 3*maxPending + 17
	for i := 0; i < extra; i++ {
		rec.record(numbered(maxPending+i, 1))
	}
	first := restartsOf(t, rec, 0)
	if want := float64(extra); first != want {
		t.Errorf("oldest kept = %v, want %v", first, want)
	}
	for i := 1; i < rec.Pending(); i++ {
		if got := restartsOf(t, rec, i); got != first+float64(i) {
			t.Fatalf("entry %d = %v, want %v", i, got, first+float64(i))
		}
	}
	if rec.Dropped() != extra {
		t.Errorf("dropped = %d, want %d", rec.Dropped(), extra)
	}
}

func restartsOf(t *testing.T, rec *Recorder, i int) float64 {
	t.Helper()
	attr := rec.pending[i].Observation.Attributes["restarts"]
	n, ok := attr.AsNumber()
	if !ok {
		t.Fatalf("entry %d has no restart count", i)
	}
	return n
}

// BenchmarkRecorderRecordQueueFull is the cost of one Submit's worth of
// observations (25, a pod's list event) while the queue is full.
func BenchmarkRecorderRecordQueueFull(b *testing.B) {
	rec, err := NewRecorder(Ring{}, "salt", newClock().Now)
	if err != nil {
		b.Fatal(err)
	}
	obs := numbered(0, 25)
	for rec.Pending() < maxPending {
		rec.record(obs)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.record(obs)
	}
}
