package pipeline

import (
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/storage"
)

// storeClock is a settable clock for the state store.
type storeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *storeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *storeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// openClockedStore opens a claimed store on clock; the test closes it.
func openClockedStore(
	t *testing.T, path string, clock *storeClock,
) *storage.Store {
	t.Helper()
	s, err := storage.Open(path, storage.Options{Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(); err != nil {
		t.Fatal(err)
	}
	return s
}

func loadFingerprints(t *testing.T, ps IncidentStore) map[string]string {
	t.Helper()
	got, err := ps.LoadFingerprints()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestIncidentStoreFingerprintsWriteAtMostOncePerInterval(t *testing.T) {
	clock := &storeClock{now: time.Unix(1_800_000_000, 0)}
	s := openClockedStore(t, t.TempDir()+"/state.db", clock)
	defer s.Close()
	ps := NewIncidentStore(s)

	if err := ps.SaveFingerprints(map[string]any{"k": "1"}); err != nil {
		t.Fatal(err)
	}
	if got := loadFingerprints(t, ps); got["k"] != "1" {
		t.Fatalf("first save must write at once, got %v", got)
	}
	clock.Advance(fingerprintInterval - time.Second)
	_ = ps.SaveFingerprints(map[string]any{"k": "2"})
	if got := loadFingerprints(t, ps); got["k"] != "1" {
		t.Fatalf("save within the interval must wait, got %v", got)
	}
	clock.Advance(time.Second)
	_ = ps.SaveFingerprints(map[string]any{"k": "3"})

	if got := loadFingerprints(t, ps); got["k"] != "3" {
		t.Fatalf("save after the interval must write, got %v", got)
	}
}

// Fingerprints held back by the interval are written when the store
// closes, so a clean shutdown loses none.
func TestIncidentStoreHeldFingerprintsAreWrittenAtClose(t *testing.T) {
	clock := &storeClock{now: time.Unix(1_800_000_000, 0)}
	path := t.TempDir() + "/state.db"
	s := openClockedStore(t, path, clock)
	ps := NewIncidentStore(s)
	_ = ps.SaveFingerprints(map[string]any{"k": "1"})
	_ = ps.SaveFingerprints(map[string]any{"k": "2", "j": "new"})

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openClockedStore(t, path, clock)
	defer reopened.Close()

	got := loadFingerprints(t, NewIncidentStore(reopened))
	if got["k"] != "2" || got["j"] != "new" {
		t.Fatalf("got %v, want the held set", got)
	}
}

func TestIncidentStoreSaveUpdatesChangedIncident(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))
	_ = ps.SaveIncidents([]incident.Record{{ID: "a"}, {ID: "b"}})

	err := ps.SaveIncidents([]incident.Record{
		{ID: "a", Revision: 2}, {ID: "b"},
	})
	got, _ := ps.LoadIncidents()

	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, r := range got {
		if r.ID == "a" && r.Revision != 2 {
			t.Fatalf("a was not rewritten: %+v", r)
		}
	}
}

func TestIncidentStoreDeleteBaselinesRemovesNamedKeys(t *testing.T) {
	d := NewIncidentStore(openTempStore(t)).(*diskIncidents)
	err := d.SaveBaselines(map[string]inventory.Baseline{
		"old": {}, "kept": {},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := d.DeleteBaselines([]string{"old"}); err != nil {
		t.Fatal(err)
	}
	got, err := d.LoadBaselines()

	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, ok := got["kept"]; !ok {
		t.Fatalf("got %v, want kept", got)
	}
}

// The writer's final write flushes held-back fingerprints itself. A
// process killed after it, before the store closes, must not leave the
// older set behind: the next start would replay recent changes as
// changes made while kwatch was down.
func TestStoreWriterFinalWriteFlushesFingerprints(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))
	if err := ps.SaveFingerprints(map[string]any{"pod/a": "1"}); err != nil {
		t.Fatal(err)
	}
	// Inside the fingerprint interval: held back, not written.
	if err := ps.SaveFingerprints(map[string]any{"pod/a": "2"}); err != nil {
		t.Fatal(err)
	}
	w := newStoreWriter(ps, func(time.Duration) <-chan time.Time {
		return nil
	}, &workerStats{})
	go w.run()

	if !w.close(nil) {
		t.Fatal("writer did not finish")
	}

	got, err := ps.LoadFingerprints()
	if err != nil || got["pod/a"] != "2" {
		t.Fatalf("fingerprints after final write = %v (%v), want the "+
			"latest set", got, err)
	}
}
