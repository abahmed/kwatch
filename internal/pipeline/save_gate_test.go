package pipeline

import (
	"testing"
	"time"
)

func TestSaveGateBuildsOneSnapshotPerBatch(t *testing.T) {
	st := &memStore{}
	e := newTestEngine(t, &lockedClock{
		now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC),
		ch:  make(chan time.Time),
	}, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Store = st
		d.Timer = func(time.Duration) <-chan time.Time {
			return make(chan time.Time)
		}
	})
	gate := &saveGate{}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	e.persist(gate, now, true)
	if gate.dirty || gate.armed != nil {
		t.Fatalf("first decision should snapshot at once: %+v", gate)
	}
	first := gate.lastSnapshot
	e.persist(gate, now.Add(100*time.Millisecond), true)
	e.persist(gate, now.Add(200*time.Millisecond), true)
	if !gate.dirty || gate.armed == nil || gate.lastSnapshot != first {
		t.Fatalf("decisions inside a batch only mark dirty: %+v", gate)
	}
	e.persist(gate, now.Add(writeBatch), false)
	if gate.dirty || gate.lastSnapshot.Equal(first) {
		t.Fatalf("dirty state should snapshot after the batch: %+v", gate)
	}
}

func TestSaveGateRefreshesIdleStateEverySaveInterval(t *testing.T) {
	e := newTestEngine(t, &lockedClock{
		now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC),
		ch:  make(chan time.Time),
	}, (&sinkLog{}).sink, func(d *Dependencies) { d.Store = &memStore{} })
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	gate := &saveGate{lastSave: now}

	e.persist(gate, now.Add(saveInterval-time.Second), false)
	if gate.lastSave != now {
		t.Fatal("idle state saved before the interval")
	}
	e.persist(gate, now.Add(saveInterval), false)
	if gate.lastSave != now.Add(saveInterval) {
		t.Fatal("idle state not refreshed after the interval")
	}
}
