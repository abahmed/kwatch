package incident

import (
	"testing"
	"time"
)

func TestManagerTakeOpenedReportsNewIncidentsOnce(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("api-1"))

	got := r.m.TakeOpened()

	if len(got) != 1 || got[0].ID != r.only().ID {
		t.Fatalf("opened = %+v, want the new incident", got)
	}
	if len(got[0].Members) != 1 {
		t.Fatalf("members = %d, want the finding that opened it",
			len(got[0].Members))
	}
	if again := r.m.TakeOpened(); len(again) != 0 {
		t.Fatalf("opened again = %+v, want nothing", again)
	}
}

func TestManagerTakeOpenedSkipsAnnouncedIncidents(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("api-1"))
	wantAction(t, r.tick(at(10*time.Minute)), Announce, "settled")

	if got := r.m.TakeOpened(); len(got) != 0 {
		t.Fatalf("opened = %+v, want only settling incidents", got)
	}
}

func TestManagerTakeOpenedSkipsGoneIncidents(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("api-1"))
	id := r.only().ID
	r.m.mu.Lock()
	delete(r.m.incidents, id)
	r.m.mu.Unlock()

	if got := r.m.TakeOpened(); len(got) != 0 {
		t.Fatalf("opened = %+v, want nothing for a merged incident", got)
	}
}

func TestManagerOpenedIsBounded(t *testing.T) {
	m := NewManager(Config{}, nil)
	for range maxOpened + 10 {
		m.noteOpened("x")
	}
	if len(m.opened) != maxOpened {
		t.Fatalf("opened = %d, want %d", len(m.opened), maxOpened)
	}
}
