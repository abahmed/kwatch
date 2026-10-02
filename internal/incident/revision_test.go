package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestManagerRevisedCauseKeepsIncidentIdentity(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	node := entity(kube.KindNode, "n1")

	r.cause(web.Entity, node, "node n1 is out of memory")
	r.apply(at(2*time.Minute), detection.Changed, web)
	wantNone(t, r.tick(at(2*time.Minute+time.Second)))
	ds := r.tick(at(2*time.Minute + DefaultReviseSettle))

	wantAction(t, ds, Update, ReasonCauseRevised)
	got := ds[0].Incident
	if got.ID != id || got.Root != node ||
		got.Revision != 2 || len(got.Members) != 1 {
		t.Fatalf("revised incident should keep its ID: %+v", got)
	}
	if len(r.m.Export()) != 1 || r.idOf(node) != id {
		t.Fatal("a revision must not open a second incident")
	}
	wantNone(t, r.tick(at(4*time.Minute)))
}

func TestManagerRevisedCauseWaitsForWhatFollows(t *testing.T) {
	r := newRig(t, Config{})
	web, api := podSig("web"), podSig("api")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	node := entity(kube.KindNode, "n1")
	r.cause(web.Entity, node, "node n1 is out of memory")
	r.apply(at(2*time.Minute), detection.Changed, web)
	// A second workload follows the node within the revise settle.
	r.cause(api.Entity, node, "node n1 is out of memory")
	r.raise(at(2*time.Minute+10*time.Second), api)
	wantNone(t, r.tick(at(2*time.Minute+10*time.Second)))

	ds := r.tick(at(2*time.Minute + DefaultReviseSettle))
	wantAction(t, ds, Update, ReasonCauseRevised)
	if got := ds[0].Incident; got.ID != id || len(got.Members) != 2 {
		t.Fatalf("one revision should carry both failures: %+v", got)
	}
	wantNone(t, r.tick(at(4*time.Minute)))
}

func TestManagerRevisedIncidentResolvesUnderItsOriginalID(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	node := entity(kube.KindNode, "n1")
	r.cause(web.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, web)
	r.tick(at(2*time.Minute + time.Second))

	r.clear(at(3*time.Minute), web)
	r.tick(at(3 * time.Minute))
	ds := r.tick(at(3*time.Minute + DefaultHold))

	wantAction(t, ds, Resolve, "healthy for 3m0s")
	if ds[0].Incident.ID != id {
		t.Fatalf("resolve id = %s", ds[0].Incident.ID)
	}
}

func TestManagerRevisionAbsorbsUnannouncedNewRoot(t *testing.T) {
	r := newRig(t, Config{})
	deploy := entity(kube.KindDeployment, "web")
	a, b := podOf(r, deploy, "web-a"), podOf(r, deploy, "web-b")
	r.raise(at(0), a, b)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	id := r.idOf(deploy)
	node := entity(kube.KindNode, "n1")

	r.cause(a.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, a)
	r.cause(b.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, b)

	ds := r.tick(at(2*time.Minute + DefaultReviseSettle))
	wantAction(t, ds, Update, ReasonCauseRevised)
	if got := ds[0].Incident; got.ID != id ||
		got.Root != node || len(got.Members) != 2 {
		t.Fatalf("incident should move with both members: %+v", got)
	}
	if n := len(r.m.Export()); n != 1 {
		t.Fatalf("incidents = %d, want 1", n)
	}
}

func TestManagerRevisionOntoAnnouncedRootSupersedes(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	api, web := podSig("api"), podSig("web")
	r.cause(api.Entity, node, "node down")
	r.raise(at(0), api, web)
	ds := r.tick(at(DefaultSettle))
	if len(ds) != 2 {
		t.Fatalf("want two announcements, got %+v", ds)
	}
	webID, nodeID := r.idOf(web.Entity), r.idOf(node)

	r.cause(web.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, web)
	ds = r.tick(at(2*time.Minute + time.Second))

	wantAction(t, ds, Resolve, ReasonSuperseded)
	got := ds[0].Incident
	if got.ID != webID || got.SupersededBy != nodeID {
		t.Fatalf("old incident should be superseded: %+v", got)
	}
	for _, e := range got.Timeline {
		if len(e.Text) >= 9 && e.Text[:9] == "recovered" {
			t.Fatalf("superseded incident claims recovery: %q", e.Text)
		}
	}
}

func TestManagerSupersedeCancelledWhenMemberReturns(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	api, web := podSig("api"), podSig("web")
	r.cause(api.Entity, node, "node down")
	r.raise(at(0), api, web)
	r.tick(at(DefaultSettle))

	r.cause(web.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, web)
	delete(r.rule.causes, web.Entity)
	r.apply(at(2*time.Minute), detection.Changed, web)

	wantNone(t, r.tick(at(2*time.Minute+time.Second)))
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if p := r.m.lookup(web.Entity); p.SupersededBy != "" ||
		p.State != Open {
		t.Fatalf("returning member must cancel supersede: %+v", p)
	}
}

func TestManagerOldRootFailingAfterRevisionGetsNewID(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	revised := r.idOf(web.Entity)
	node := entity(kube.KindNode, "n1")
	r.cause(web.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, web)
	r.tick(at(2*time.Minute + time.Second))

	delete(r.rule.causes, web.Entity)
	r.clear(at(3*time.Minute), web)
	oom := sig(web.Entity, "OOMKilled", detection.Warning)
	r.raise(at(3*time.Minute), oom)

	fresh := r.idOf(web.Entity)
	if fresh == revised || r.idOf(node) != revised {
		t.Fatalf("old root should get a fresh incident: %s, revised %s",
			fresh, revised)
	}
	if got := r.of(web.Entity).Previous; got != "" {
		t.Fatalf("a revised incident is not a recurrence: %q", got)
	}
}

func podOf(r *rig, owner inventory.EntityID, name string) detection.Finding {
	s := podSig(name)
	r.relate(s.Entity, inventory.OwnedBy, owner)
	return s
}
