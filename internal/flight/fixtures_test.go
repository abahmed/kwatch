package flight

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/storage"
)

var base = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)

// clockAt is a clock the test moves.
type clockAt struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clockAt) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clockAt) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newClock() *clockAt { return &clockAt{now: base} }

func openStore(
	t *testing.T, path string, now func() time.Time,
) *storage.Store {
	t.Helper()
	s, err := storage.Open(path, storage.Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Claim(); err != nil {
		t.Fatal(err)
	}
	return s
}

// newRecorder returns a recorder on a fresh store, and the ring.
func newRecorder(
	t *testing.T, clock *clockAt,
) (*Recorder, Ring, *storage.Store) {
	t.Helper()
	store := openStore(t, t.TempDir()+"/state.db", clock.Now)
	salt, err := LoadSalt(store)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewRecorder(NewRing(store), salt, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return rec, NewRing(store), store
}

func podObserved(name string, restarts float64) inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Observed, Source: "kubernetes",
		Entity: inventory.CoreID("pod", "shop", name),
		Attributes: map[string]inventory.Value{
			"restarts": inventory.Number(restarts),
			"reason":   inventory.Text("CrashLoopBackOff"),
			"message":  inventory.Text("panic: db password is hunter2"),
		},
	}
}

func submitAll(rec *Recorder, obs ...inventory.Observation) {
	rec.Wrap(func(context.Context, ...inventory.Observation) {})(
		context.Background(), obs...)
}

func readAll(t *testing.T, ring Ring) []replay.Entry {
	t.Helper()
	var out []replay.Entry
	err := ring.Walk(time.Time{}, time.Time{}, func(e replay.Entry) error {
		out = append(out, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
