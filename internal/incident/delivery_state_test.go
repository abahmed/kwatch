package incident

import (
	"testing"
	"time"
)

func TestManagerRecordScopeIsKeptUntilRecurrence(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)

	r.m.RecordScope(id, true)
	r.m.RecordScope("missing", true)
	if got := r.only().Scope; got != ScopeIn {
		t.Fatalf("scope = %v, want in", got)
	}
	r.m.RecordScope(id, false)
	if got := r.only().Scope; got != ScopeOut {
		t.Fatalf("scope = %v, want out", got)
	}

	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	r.tick(at(2*time.Minute + DefaultHold))
	r.raise(at(time.Hour), web)
	if got := r.of(web.Entity).Scope; got != ScopeUnknown {
		t.Fatalf("recurrence scope = %v, want unknown", got)
	}
}

func TestManagerRestoreReannouncesHeldIncident(t *testing.T) {
	old := newRig(t, Config{})
	web := podSig("web")
	announced(t, old, web)
	old.m.HoldAnnouncement(old.idOf(web.Entity))
	records := old.m.Export()
	if !records[0].Held {
		t.Fatal("held flag must be persisted")
	}

	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(10*time.Minute))
	if got := fresh.only(); got.State != Settling ||
		!got.Announced.IsZero() {
		t.Fatalf("held incident must restore unannounced: %+v", got)
	}
	fresh.raise(at(5*time.Minute), web)
	ds := fresh.tick(at(5*time.Minute + time.Second))
	wantAction(t, ds, Announce, "settled")
	if ds[0].Incident.Revision != 2 {
		t.Fatalf("revision = %d, want 2", ds[0].Incident.Revision)
	}
}

func TestManagerReleasedAnnouncementRestoresAsOpen(t *testing.T) {
	old := newRig(t, Config{})
	web := podSig("web")
	announced(t, old, web)
	old.m.HoldAnnouncement(old.idOf(web.Entity))
	old.m.ReleaseAnnouncement(old.idOf(web.Entity))

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))
	if got := fresh.only(); got.State != Open || got.Held {
		t.Fatalf("released incident must stay open: %+v", got)
	}
}

func TestManagerRestoreTreatsLegacyAnnouncedRecordAsInScope(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	records := old.m.Export()
	records[0].Scope = ScopeUnknown

	fresh := newRig(t, Config{})
	fresh.m.Restore(records, time.Time{})
	if got := fresh.only().Scope; got != ScopeIn {
		t.Fatalf("scope = %v, want in", got)
	}
}

func TestManagerClosedReportsLiveIncidents(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)

	if r.m.Closed([]string{id}) {
		t.Fatal("open incident is not closed")
	}
	if !r.m.Closed([]string{"forgotten"}) || !r.m.Closed(nil) {
		t.Fatal("unknown incidents count as closed")
	}
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	r.tick(at(2*time.Minute + DefaultHold))
	if !r.m.Closed([]string{id}) {
		t.Fatal("resolved incident is closed")
	}
}
