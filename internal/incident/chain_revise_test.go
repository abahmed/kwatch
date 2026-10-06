package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A cascade is announced from its second step, the API, because the
// failure of the database behind it is only reported later. The database
// began first, so the solver now roots the API's failure at it with the
// chain between them: the announced incident is revised and moves to the
// database, and keeps the chain.
func TestManagerChainRootedLaterReRootsAnnouncedIncident(t *testing.T) {
	r := newRig(t, Config{})
	api := podSig("api")
	announced(t, r, api)
	id := r.idOf(api.Entity)
	db := entity(kube.KindDeployment, "postgres")
	cause := r.cause(api.Entity, db, "postgres runs out of memory")
	cause.Began = at(-time.Minute)
	cause.Hops = []explain.Hop{
		{Entity: db, Began: at(-time.Minute)},
		{Entity: entity(kube.KindDeployment, "api"), Began: at(0),
			Link: explain.LinkCalls}}
	r.rule.causes[api.Entity] = cause

	r.apply(at(2*time.Minute), detection.Changed, api)

	if r.idOf(db) != id {
		t.Fatalf("the incident must move to the database, got %s",
			r.idOf(db))
	}
	got := r.of(db).Cause
	if got == nil || len(got.Hops) != 2 || got.Hops[0] != (rootcause.Hop{
		Entity: db, Began: at(-time.Minute)}) {
		t.Fatalf("cause = %+v, want the chain kept", got)
	}
}

// A chain whose root began long after the announcement does not take
// the incident over: the chain is not a license to blame something that
// failed afterwards.
func TestManagerChainRootedTooLateKeepsAnnouncedIncident(t *testing.T) {
	r := newRig(t, Config{})
	api := podSig("api")
	announced(t, r, api)
	id := r.idOf(api.Entity)
	db := entity(kube.KindDeployment, "postgres")
	cause := r.cause(api.Entity, db, "postgres runs out of memory")
	cause.Began = at(30 * time.Minute)
	cause.Hops = []explain.Hop{{Entity: db, Began: at(30 * time.Minute)}}
	r.rule.causes[api.Entity] = cause

	r.apply(at(31*time.Minute), detection.Changed, api)

	if r.idOf(api.Entity) != id {
		t.Fatal("the announced incident must keep its root")
	}
}
