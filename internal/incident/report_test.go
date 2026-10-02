package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// newSince returns the entries an update names as new.
func newSince(p Incident) []Event {
	if !p.ReportedKnown {
		return nil
	}
	return p.Timeline[p.Reported:]
}

// Entries noted in the same second as the last delivered message are
// still new: the mark counts entries instead of comparing times.
func TestManagerUpdateNamesEntriesNotedInTheSameSecond(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	r.m.RecordScope(id, true)
	reported := len(r.of(web.Entity).Timeline)

	// Same timestamp as the announcement.
	r.raise(at(DefaultSettle), sig(web.Entity,
		reasons.OOMKilled, detection.Warning))
	ds := r.tick(at(DefaultSettle + time.Minute))
	wantAction(t, ds, Update, "material change")

	p := ds[0].Incident
	if !p.ReportedKnown || p.Reported != reported {
		t.Fatalf("reported = %d (known %v), want %d",
			p.Reported, p.ReportedKnown, reported)
	}
	if len(newSince(p)) == 0 {
		t.Fatalf("update names nothing new: %+v", p.Timeline)
	}
}

// An announcement dropped as out of scope reached nobody, so the next
// message must still report everything.
func TestManagerOutOfScopeAnnouncementKeepsSentMark(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	r.m.RecordScope(r.idOf(web.Entity), false)

	r.raise(at(2*time.Minute), sig(web.Entity,
		reasons.OOMKilled, detection.Warning))
	ds := r.tick(at(3 * time.Minute))
	wantAction(t, ds, Update, "material change")
	if p := ds[0].Incident; !p.ReportedKnown || p.Reported != 0 {
		t.Fatalf("reported = %d, want 0: nobody got the announcement",
			p.Reported)
	}
}

// A held announcement moves the mark only when it is released.
func TestManagerHeldAnnouncementMovesSentMarkOnRelease(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	r.m.RecordScope(id, true)
	r.m.HoldAnnouncement(id)
	if p := r.of(web.Entity); p.Reported != 0 {
		t.Fatalf("held: reported = %d, want 0", p.Reported)
	}

	r.m.ReleaseAnnouncement(id)
	p := r.of(web.Entity)
	if p.Reported == 0 || p.Reported != len(p.Timeline) {
		t.Fatalf("released: reported = %d of %d entries",
			p.Reported, len(p.Timeline))
	}
}

func TestManagerRestoredIncidentHasUnknownSentMark(t *testing.T) {
	src := newRig(t, Config{})
	web := podSig("web")
	announced(t, src, web)

	dst := newRig(t, Config{})
	dst.m.Restore(src.m.Export(), time.Time{})
	if p := dst.of(web.Entity); p.ReportedKnown {
		t.Fatalf("restored mark must be unknown: %d", p.Reported)
	}
}

// Trimming the timeline's front moves the mark with it.
func TestSentMarkFollowsTrimmedTimeline(t *testing.T) {
	p := &Incident{}
	for i := 0; i < 10; i++ {
		p.note(at(0), "early")
	}
	p.sent.decided(true)
	for i := 0; i < 60; i++ {
		p.note(at(time.Minute), "late")
	}
	got, known := p.sent.reported(len(p.Timeline))
	if !known || got != 0 {
		t.Fatalf("reported = %d, want 0 after the reported part was trimmed",
			got)
	}
	p.sent.decided(true)
	if got, _ := p.sent.reported(len(p.Timeline)); got != len(p.Timeline) {
		t.Fatalf("reported = %d, want all %d", got, len(p.Timeline))
	}
}
