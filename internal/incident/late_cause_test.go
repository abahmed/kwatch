package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A cause that began long after an incident was announced did not cause
// what people were told about: the incident keeps its root.
func TestManagerLateCauseDoesNotTakeOverAnnouncedIncident(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	node := entity(kube.KindNode, "n1")
	cause := r.cause(web.Entity, node, "node n1 is out of memory")
	cause.Began = at(30 * time.Minute)
	r.rule.causes[web.Entity] = cause

	r.apply(at(31*time.Minute), detection.Changed, web)

	if r.idOf(web.Entity) != id {
		t.Fatal("the announced incident must keep its root")
	}
	if p := r.of(web.Entity); len(p.Members) != 1 || p.Cause != nil {
		t.Fatalf("incident = %+v, want its member kept and no cause", p)
	}
}

// A cause that began within the window after the announcement is a
// revised cause, as before.
func TestManagerEarlyCauseStillRevisesAnnouncedIncident(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.idOf(web.Entity)
	node := entity(kube.KindNode, "n1")
	cause := r.cause(web.Entity, node, "node n1 is out of memory")
	cause.Began = at(explain.TemporalExclusion / 2)
	r.rule.causes[web.Entity] = cause

	r.apply(at(2*time.Minute), detection.Changed, web)

	if r.idOf(node) != id {
		t.Fatalf("the incident must move to the node, got %s", r.idOf(node))
	}
}
