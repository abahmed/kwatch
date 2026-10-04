package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A group whose members go back to their own workloads is over: its
// conversation does not continue under whichever member left last.
func TestManagerDispersedMembersEndTheIncident(t *testing.T) {
	r := newRig(t, Config{})
	web, api := podSig("web"), podSig("api")
	pool := entity(kube.KindNodePool, "pool")
	r.cause(web.Entity, pool, "node pool pool is failing")
	r.cause(api.Entity, pool, "node pool pool is failing")
	r.raise(at(0), web, api)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	id := r.idOf(pool)

	// The pool recovers: each workload is explained by itself again.
	delete(r.rule.causes, web.Entity)
	delete(r.rule.causes, api.Entity)
	r.apply(at(2*time.Minute), detection.Changed, web, api)

	if r.idOf(pool) != id || r.of(pool).State != Open ||
		len(r.of(pool).Members) != 0 {
		t.Fatalf("the pool incident must stay, empty: %+v", r.of(pool))
	}
	for _, s := range []detection.Finding{web, api} {
		if p := r.of(s.Entity); p.ID == id || p.State != Settling {
			t.Fatalf("%s must start its own incident, got %+v",
				s.Entity, p)
		}
	}
	// It recovers like any incident whose failures cleared.
	wantNone(t, r.tick(at(2*time.Minute+time.Second)))
	ds := r.tick(at(2*time.Minute + DefaultHold + 2*time.Second))
	if len(ds) == 0 {
		t.Fatal("the empty pool incident must resolve after the hold")
	}
	for _, d := range ds {
		if d.Incident.ID == id && d.Action != Resolve {
			t.Fatalf("pool incident decision = %+v, want a resolve", d)
		}
	}
}

// Members that all leave for the same root move the story there, as
// before: a revised cause keeps the incident's identity.
func TestManagerMembersLeavingTogetherMoveTheIncident(t *testing.T) {
	r := newRig(t, Config{})
	web, api := podSig("web"), podSig("api")
	node := entity(kube.KindNode, "n1")
	announced(t, r, web)
	announced(t, r, api)
	id := r.idOf(web.Entity)

	r.cause(web.Entity, node, "node n1 is out of memory")
	r.cause(api.Entity, node, "node n1 is out of memory")
	r.apply(at(2*time.Minute), detection.Changed, web, api)

	if r.idOf(node) != id {
		t.Fatalf("the incident must move to the node, ids: %s vs %s",
			r.idOf(node), id)
	}
}
