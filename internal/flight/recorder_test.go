package flight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/storage"
)

func TestWrapForwardsObservationsUntouched(t *testing.T) {
	rec, _, _ := newRecorder(t, newClock())
	var got []inventory.Observation
	submit := rec.Wrap(func(_ context.Context, o ...inventory.Observation) {
		got = append(got, o...)
	})
	in := podObserved("api-1", 1)
	submit(context.Background(), in)
	if len(got) != 1 || got[0].Entity.Name != "api-1" ||
		got[0].Attributes["message"].AsText() == "" {
		t.Errorf("the engine must receive the original, got %+v", got)
	}
}

func TestRecordedObservationsAreSanitized(t *testing.T) {
	clock := newClock()
	rec, ring, _ := newRecorder(t, clock)
	submitAll(rec, podObserved("api-1", 3))
	rec.Flush()
	entries := readAll(t, ring)
	if len(entries) != 1 {
		t.Fatalf("recorded %d entries, want 1", len(entries))
	}
	data, _ := json.Marshal(entries)
	for _, leak := range []string{"api-1", "shop", "hunter2", "panic"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("%q leaked into the recording: %s", leak, data)
		}
	}
	o := entries[0].Observation
	if o.Attributes["reason"].AsText() != "CrashLoopBackOff" {
		t.Error("reason codes must be kept")
	}
	if n, _ := o.Attributes["restarts"].AsNumber(); n != 3 {
		t.Error("numbers must be kept")
	}
}

func TestSecretsNeverReachTheRing(t *testing.T) {
	rec, ring, _ := newRecorder(t, newClock())
	secret := inventory.Observation{
		Kind: inventory.Observed, Source: "kubernetes",
		Entity: inventory.CoreID("secret", "billing", "Pull"),
		Attributes: map[string]inventory.Value{
			"keys": inventory.Text("Token"), "digest": inventory.Text("Abc"),
		},
	}
	submitAll(rec, secret)
	rec.Flush()
	data, _ := json.Marshal(readAll(t, ring))
	for _, leak := range []string{"Token", "Abc", "Pull", "billing"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("%q leaked: %s", leak, data)
		}
	}
}

func TestSamePseudonymsAcrossBatchesAndRestarts(t *testing.T) {
	clock := newClock()
	path := t.TempDir() + "/state.db"
	store := openStore(t, path, clock.Now)
	salt1, _ := LoadSalt(store)
	salt2, _ := LoadSalt(store)
	if salt1 == "" || salt1 != salt2 {
		t.Fatalf("the salt must be created once: %q %q", salt1, salt2)
	}
	a, _ := NewRecorder(NewRing(store), salt1, clock.Now)
	submitAll(a, podObserved("api-1", 1))
	a.Flush()
	b, _ := NewRecorder(NewRing(store), salt2, clock.Now)
	clock.advance(time.Minute)
	submitAll(b, podObserved("api-1", 2))
	b.Flush()
	entries := readAll(t, NewRing(store))
	if len(entries) != 2 ||
		entries[0].Observation.Entity != entries[1].Observation.Entity {
		t.Errorf("one pod must keep one pseudonym: %+v", entries)
	}
}

func TestIdenticalObservationsAreRecordedOnceUntilRefresh(t *testing.T) {
	clock := newClock()
	rec, ring, _ := newRecorder(t, clock)
	for i := 0; i < 5; i++ {
		clock.advance(time.Second)
		submitAll(rec, podObserved("api-1", 1))
	}
	rec.Flush()
	if got := len(readAll(t, ring)); got != 1 {
		t.Fatalf("recorded %d copies of one state, want 1", got)
	}
	submitAll(rec, podObserved("api-1", 2))
	clock.advance(refreshEvery + time.Second)
	submitAll(rec, podObserved("api-1", 2))
	rec.Flush()
	if got := len(readAll(t, ring)); got != 3 {
		t.Errorf("a change and a refresh must both be recorded, got %d", got)
	}
}

func TestEntriesNeverGoBackInTime(t *testing.T) {
	clock := newClock()
	rec, ring, _ := newRecorder(t, clock)
	submitAll(rec, podObserved("a", 1))
	clock.advance(-time.Hour) // the wall clock stepped back
	submitAll(rec, podObserved("b", 1))
	rec.Flush()
	entries := readAll(t, ring)
	if len(entries) != 2 || entries[1].At.Before(entries[0].At) {
		t.Errorf("entries out of order: %+v", entries)
	}
}

func TestQueueIsBoundedAndDropsTheOldest(t *testing.T) {
	clock := newClock()
	rec, ring, _ := newRecorder(t, clock)
	for i := 0; i < maxPending+500; i++ {
		clock.advance(time.Millisecond)
		submitAll(rec, podObserved(fmt.Sprintf("p-%d", i), float64(i)))
	}
	if rec.Pending() > maxPending {
		t.Fatalf("pending = %d, bound is %d", rec.Pending(), maxPending)
	}
	rec.Flush()
	entries := readAll(t, ring)
	if len(entries) != maxPending || rec.Dropped() != 500 {
		t.Errorf("recorded %d, dropped %d; want %d and 500",
			len(entries), rec.Dropped(), maxPending)
	}
	first := entries[0].Observation.Attributes["restarts"]
	if n, _ := first.AsNumber(); n != 500 {
		t.Errorf("the oldest must go first, first kept is %v", n)
	}
}

func TestFlushWritesBatchesOfBoundedSize(t *testing.T) {
	rec, ring, store := newRecorder(t, newClock())
	for i := 0; i < maxBatchEntries+10; i++ {
		submitAll(rec, podObserved(fmt.Sprintf("p-%d", i), 1))
	}
	rec.Flush()
	batches := 0
	_ = storage.FlightLog[Batch](store).Range(Stream, time.Time{},
		time.Time{}, func(_ time.Time, b Batch) error {
			batches++
			if b.N > maxBatchEntries {
				t.Errorf("batch of %d entries", b.N)
			}
			return nil
		})
	if batches != 2 || len(readAll(t, ring)) != maxBatchEntries+10 {
		t.Errorf("batches = %d", batches)
	}
}

func TestStoreFailureNeverBlocksOrGrowsMemory(t *testing.T) {
	clock := newClock()
	rec, _, store := newRecorder(t, clock)
	_ = store.Close() // every write now fails
	for i := 0; i < 3*maxPending; i++ {
		submitAll(rec, podObserved(fmt.Sprintf("p-%d", i), 1))
		if i%1000 == 0 {
			rec.Flush()
		}
	}
	rec.Flush()
	if rec.Pending() != 0 {
		t.Errorf("a failed write must not keep growing the queue: %d",
			rec.Pending())
	}
}

func TestFailedWriteDoesNotHideTheNextObservation(t *testing.T) {
	clock := newClock()
	rec, _, store := newRecorder(t, clock)
	_ = store.Close() // the write fails
	submitAll(rec, podObserved("api-1", 1))
	rec.Flush()
	if len(rec.seen) != 0 {
		t.Errorf("a state that was never written is remembered: %d",
			len(rec.seen))
	}
}
