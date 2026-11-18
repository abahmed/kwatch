package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// twoWorkloadsFixture is a Deployment api, which sorts first, whose
// revision 2 before its newest 3 served, and a Deployment web, whose
// revision 5 before its newest 7 never became ready.
func twoWorkloadsFixture(t *testing.T) (*fixture, inventory.EntityID,
	inventory.EntityID) {
	f := newFixture(t)
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(api, web)
	for _, rs := range []struct {
		owner          inventory.EntityID
		name, revision string
		ready          float64
	}{
		{api, "api-3", "3", 0}, {api, "api-2", "2", 2},
		{web, "web-7", "7", 0}, {web, "web-5", "5", 0},
	} {
		id := inventory.CoreID(kube.KindReplicaSet, "shop", rs.name)
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: id, Attributes: map[string]inventory.Value{
				kube.AttrRevision:      inventory.Text(rs.revision),
				kube.AttrReadyReplicas: inventory.Number(rs.ready)}})
		f.relate(id, inventory.OwnedBy, rs.owner)
	}
	return f, api, web
}

// A cause that covers two workloads blames the change of one: the
// revision a rollback returns to and whether it served are that
// workload's, the one the undo names, never those of another covered
// workload that sorts first. A change of no covered workload has none.
func TestRecordReadsTheBlamedWorkloadsRollback(t *testing.T) {
	f, api, web := twoWorkloadsFixture(t)
	covers := []inventory.EntityID{api, web}
	s := f.snapshot()

	got := s.Record(Cause{Root: web, Covers: covers,
		Changes: []inventory.Change{{Entity: web, At: t0}}})
	if got.RollbackRevision != "5" || got.PreviousServed {
		t.Fatalf("web blamed: revision %q, served %v; want 5, false",
			got.RollbackRevision, got.PreviousServed)
	}

	got = s.Record(Cause{Root: api, Covers: covers,
		Changes: []inventory.Change{{Entity: api, At: t0}}})
	if got.RollbackRevision != "2" || !got.PreviousServed {
		t.Fatalf("api blamed: revision %q, served %v; want 2, true",
			got.RollbackRevision, got.PreviousServed)
	}

	secret := inventory.CoreID(kube.KindSecret, "shop", "keys")
	got = s.Record(Cause{Root: web, Covers: covers,
		Changes: []inventory.Change{{Entity: secret, At: t0}}})
	if got.RollbackRevision != "" || got.PreviousServed {
		t.Fatalf("secret blamed: revision %q, served %v; want none",
			got.RollbackRevision, got.PreviousServed)
	}
}
