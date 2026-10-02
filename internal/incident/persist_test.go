package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestManagerRestoreDoesNotReannounce(t *testing.T) {
	old := newRig(t, Config{})
	web := podSig("web")
	announced(t, old, web)
	records := old.m.Export()

	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(10*time.Minute))
	fresh.raise(at(5*time.Minute), web)

	wantNone(t, fresh.tick(at(6*time.Minute)))
	got := fresh.only()
	if got.State != Open || got.Revision != 1 || got.Digest == "" {
		t.Fatalf("restored incident changed: %+v", got)
	}
}

func TestManagerRestoreGraceDelaysRecovery(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))

	wantNone(t, fresh.tick(at(5*time.Minute)))
	if fresh.only().State != Open {
		t.Fatal("incident must stay open during the grace period")
	}
	wantNone(t, fresh.tick(at(10*time.Minute)))
	if fresh.only().State != Recovering {
		t.Fatal("incident should recover once grace has passed")
	}
	ds := fresh.tick(at(10*time.Minute + DefaultHold))
	wantAction(t, ds, Resolve, "healthy for 3m0s")
}

func TestManagerRestoreGraceHoldsSettlingIncidents(t *testing.T) {
	old := newRig(t, Config{})
	old.raise(at(0), podSig("web"))

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))
	wantNone(t, fresh.tick(at(5*time.Minute)))
	if fresh.only().State != Settling {
		t.Fatal("settling incident must wait for findings to return")
	}
	wantNone(t, fresh.tick(at(10*time.Minute)))
	if fresh.only().State != Resolved {
		t.Fatal("still no findings after grace: resolve silently")
	}
}

func TestManagerExportRestoreRoundTrip(t *testing.T) {
	src := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	pod := podSig("web")
	src.cause(pod.Entity, node, "node down")
	src.raise(at(0), pod)
	src.tick(at(DefaultPageSettle + DefaultSettle))
	want := src.m.Export()

	dst := newRig(t, Config{})
	dst.m.Restore(want, time.Time{})
	got := dst.m.Export()
	if len(got) != 1 || got[0].ID != want[0].ID ||
		got[0].Cause == nil || got[0].Cause.Summary != "node down" ||
		got[0].Tier != want[0].Tier || got[0].State != want[0].State ||
		len(got[0].Timeline) != len(want[0].Timeline) {
		t.Fatalf("round trip differs:\n%+v\n%+v", got, want)
	}
}

func TestManagerExportIsSortedAndDetached(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("b"), podSig("a"))
	records := r.m.Export()
	if len(records) != 2 || records[0].ID > records[1].ID {
		t.Fatalf("records not sorted: %+v", records)
	}
	records[0].Timeline[0].Text = "tampered"
	if r.m.Export()[0].Timeline[0].Text == "tampered" {
		t.Fatal("export must not alias internal timeline")
	}
}

func TestIncidentSnapshotIsDetached(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	pod := podSig("web")
	r.cause(pod.Entity, node, "node down")
	r.raise(at(0), pod)
	r.m.mu.Lock()
	p := r.m.lookup(node)
	p.Cycles = []time.Time{at(0)}
	r.m.mu.Unlock()

	snap := p.Snapshot()
	snap.Members[detection.Key{}] = detection.Finding{}
	snap.Impact = append(snap.Impact, node)
	snap.Cycles[0] = at(time.Hour)
	snap.Occurrences[0] = at(time.Hour)
	snap.Timeline[0].Text = "tampered"
	snap.Cause.Summary = "tampered"

	if len(p.Members) != 1 || len(p.Impact) != len(snap.Impact)-1 ||
		p.Cycles[0] != at(0) || p.Occurrences[0] != at(0) ||
		p.Timeline[0].Text == "tampered" ||
		p.Cause.Summary != "node down" {
		t.Fatalf("snapshot aliases the incident: %+v", p)
	}
}

func TestDecisionIncidentIsDetachedFromManager(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	ds := r.tick(at(DefaultSettle))
	ds[0].Incident.Members[detection.Key{}] = detection.Finding{}
	if len(r.only().Members) != 1 {
		t.Fatal("decision must carry a detached snapshot")
	}
}

// A restored recovering incident has no members until detectors run
// again, so it must not resolve inside the restore grace even when its
// hold has already passed.
func TestManagerRestoreGraceHoldsRecoveringIncidents(t *testing.T) {
	old := newRig(t, Config{})
	web := podSig("web")
	announced(t, old, web)
	old.clear(at(DefaultSettle+time.Second), web)
	wantNone(t, old.tick(at(DefaultSettle+time.Second)))
	if old.only().State != Recovering {
		t.Fatal("setup: incident should be recovering")
	}

	fresh := newRig(t, Config{})
	grace := at(30 * time.Minute)
	fresh.m.Restore(old.m.Export(), grace)
	_, next := fresh.m.Tick(at(20 * time.Minute))
	if next != 10*time.Minute {
		t.Fatalf("next wake = %v, want the grace end", next)
	}
	if fresh.only().State != Recovering {
		t.Fatal("recovering incident must not resolve during grace")
	}
	wantAction(t, fresh.tick(grace), Resolve, "healthy for 3m0s")
}

func TestManagerRestoreGraceHoldsFlappingIncidents(t *testing.T) {
	old := newRig(t, Config{})
	web := podSig("web")
	announced(t, old, web)
	records := old.m.Export()
	records[0].State = Flapping
	records[0].RecoveringSince = at(DefaultSettle)

	fresh := newRig(t, Config{})
	grace := at(DefaultMaxHold + time.Hour)
	fresh.m.Restore(records, grace)
	wantNone(t, fresh.tick(at(DefaultMaxHold+30*time.Minute)))
	if fresh.only().State != Flapping {
		t.Fatal("flapping incident must not resolve during grace")
	}
	ds := fresh.tick(grace)
	wantAction(t, ds, Resolve, "stable for "+DefaultMaxHold.String())
}

// The impact peak and a pending cause revision survive a restart, so the
// restored incident neither re-reports impact nor drops the update.
func TestManagerRestoreKeepsImpactPeakAndPendingRevision(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	p := old.m.incidents[old.idOf(entity(kube.KindPod, "web"))]
	p.impactPeak, p.revised, p.revisedAt = 5, true, at(time.Minute)

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), time.Time{})
	got := fresh.m.incidents[p.ID]
	if got.impactPeak != 5 || !got.revised ||
		!got.revisedAt.Equal(at(time.Minute)) {
		t.Fatalf("restored peak=%d revised=%v at %v", got.impactPeak,
			got.revised, got.revisedAt)
	}
}

// A digest stored by an older kwatch, with another formula or other
// fields, must not cause an update after the upgrade: the first ticks
// after the restore adopt the current digest. Real changes afterwards
// still update.
func TestManagerRestoreAdoptsCurrentDigestBeforeFirstTick(t *testing.T) {
	for name, grace := range map[string]time.Duration{
		"without grace": 0, "with grace": 10 * time.Minute,
	} {
		t.Run(name, func(t *testing.T) {
			old := newRig(t, Config{})
			web := podSig("web")
			announced(t, old, web)
			records := old.m.Export()
			records[0].Digest = "digest-of-an-older-formula"

			fresh := newRig(t, Config{})
			graceUntil := time.Time{}
			if grace > 0 {
				graceUntil = at(grace)
			}
			fresh.m.Restore(records, graceUntil)
			fresh.raise(at(time.Minute), web)
			wantNone(t, fresh.tick(at(2*time.Minute)))
			wantNone(t, fresh.tick(at(grace+3*time.Minute)))

			fresh.raise(at(grace+4*time.Minute), sig(web.Entity,
				reasons.OOMKilled, detection.Warning))
			wantAction(t, fresh.tick(at(grace+5*time.Minute)),
				Update, "material change")
		})
	}
}

// A cold start restores nothing live, so there is nothing to wait for:
// a new incident recovers on its usual hold.
func TestManagerRestoreWithoutLiveRecordsHasNoGrace(t *testing.T) {
	r := newRig(t, Config{})
	r.m.Restore(nil, at(10*time.Minute))
	web := podSig("web")
	announced(t, r, web)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))

	ds := r.tick(at(2*time.Minute + DefaultHold))
	wantAction(t, ds, Resolve, "healthy for "+DefaultHold.String())
}

func TestManagerRestoreRecoveringReturnAfterGraceIsSilent(t *testing.T) {
	old := newRig(t, Config{})
	web := podSig("web")
	announced(t, old, web)
	old.clear(at(DefaultSettle+time.Second), web)
	wantNone(t, old.tick(at(DefaultSettle+time.Second)))
	if old.only().State != Recovering {
		t.Fatal("setup: incident should be recovering")
	}

	fresh := newRig(t, Config{})
	grace := at(10 * time.Minute)
	fresh.m.Restore(old.m.Export(), grace)
	wantNone(t, fresh.tick(at(5*time.Minute)))

	fresh.raise(grace, web)
	wantNone(t, fresh.tick(grace))
	if fresh.only().State != Open {
		t.Fatal("returning members should reopen the incident")
	}
	wantNone(t, fresh.tick(grace.Add(time.Minute)))
}
