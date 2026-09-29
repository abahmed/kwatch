package problem

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
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
		t.Fatalf("restored problem changed: %+v", got)
	}
}

func TestManagerRestoreGraceDelaysRecovery(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))

	wantNone(t, fresh.tick(at(5*time.Minute)))
	if fresh.only().State != Open {
		t.Fatal("problem must stay open during the grace period")
	}
	wantNone(t, fresh.tick(at(10*time.Minute)))
	if fresh.only().State != Recovering {
		t.Fatal("problem should recover once grace has passed")
	}
	ds := fresh.tick(at(10*time.Minute + DefaultHold))
	wantAction(t, ds, Resolve, "healthy for 3m0s")
}

func TestManagerRestoreGraceHoldsSettlingProblems(t *testing.T) {
	old := newRig(t, Config{})
	old.raise(at(0), podSig("web"))

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))
	wantNone(t, fresh.tick(at(5*time.Minute)))
	if fresh.only().State != Settling {
		t.Fatal("settling problem must wait for signals to return")
	}
	wantNone(t, fresh.tick(at(10*time.Minute)))
	if fresh.only().State != Resolved {
		t.Fatal("still no signals after grace: resolve silently")
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

func TestProblemSnapshotIsDetached(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	pod := podSig("web")
	r.cause(pod.Entity, node, "node down")
	r.raise(at(0), pod)
	r.m.mu.Lock()
	p := r.m.problems[node.String()]
	p.Cycles = []time.Time{at(0)}
	r.m.mu.Unlock()

	snap := p.Snapshot()
	snap.Members[signal.Key{}] = signal.Signal{}
	snap.Impact = append(snap.Impact, node)
	snap.Cycles[0] = at(time.Hour)
	snap.Occurrences[0] = at(time.Hour)
	snap.Timeline[0].Text = "tampered"
	snap.Cause.Summary = "tampered"

	if len(p.Members) != 1 || len(p.Impact) != len(snap.Impact)-1 ||
		p.Cycles[0] != at(0) || p.Occurrences[0] != at(0) ||
		p.Timeline[0].Text == "tampered" ||
		p.Cause.Summary != "node down" {
		t.Fatalf("snapshot aliases the problem: %+v", p)
	}
}

func TestDecisionProblemIsDetachedFromManager(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	ds := r.tick(at(DefaultSettle))
	ds[0].Problem.Members[signal.Key{}] = signal.Signal{}
	if len(r.only().Members) != 1 {
		t.Fatal("decision must carry a detached snapshot")
	}
}
