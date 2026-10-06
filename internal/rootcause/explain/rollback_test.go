package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A cause covering only a Deployment finding still names the revision
// before the newest ReplicaSet.
func TestRollbackRevisionFromDeploymentCover(t *testing.T) {
	f := newFixture(t)
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(dep)
	for name, revision := range map[string]string{
		"web-7": "7", "web-5": "5", "web-3": "3",
	} {
		rs := inventory.CoreID(kube.KindReplicaSet, "shop", name)
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: rs, Attributes: map[string]inventory.Value{
				kube.AttrRevision: inventory.Text(revision)}})
		f.relate(rs, inventory.OwnedBy, dep)
	}
	got := rollbackRevision(f.snapshot().Model, []inventory.EntityID{dep})
	if got != "5" {
		t.Fatalf("rollback revision = %q, want 5", got)
	}
}

// A Deployment with a single ReplicaSet has nothing to roll back to.
func TestRollbackRevisionNeedsAnEarlierRevision(t *testing.T) {
	f := newFixture(t)
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(dep)
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "web-1")
	f.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: rs, Attributes: map[string]inventory.Value{
			kube.AttrRevision: inventory.Text("1")}})
	f.relate(rs, inventory.OwnedBy, dep)
	got := rollbackRevision(f.snapshot().Model, []inventory.EntityID{dep})
	if got != "" {
		t.Fatalf("rollback revision = %q, want none", got)
	}
}

// revisionsFixture is a Deployment with ReplicaSets at revisions 7, 5
// and 3, and one pod under each of the revisions 7 and 5 and one pod
// that no controller owns.
func revisionsFixture(f *fixture) (bare, old, newest inventory.EntityID) {
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(dep)
	pods := map[string]inventory.EntityID{}
	for name, revision := range map[string]string{
		"web-7": "7", "web-5": "5", "web-3": "3",
	} {
		rs := inventory.CoreID(kube.KindReplicaSet, "shop", name)
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: rs, Attributes: map[string]inventory.Value{
				kube.AttrRevision: inventory.Text(revision)}})
		f.relate(rs, inventory.OwnedBy, dep)
		pod := inventory.CoreID(kube.KindPod, "shop", name+"-x")
		f.add(pod)
		f.relate(pod, inventory.OwnedBy, rs)
		pods[name] = pod
	}
	bare = inventory.CoreID(kube.KindPod, "shop", "static")
	f.add(bare)
	return bare, pods["web-5"], pods["web-7"]
}

// The rollback target is the revision before the Deployment's newest,
// even when the covered pod still runs an older ReplicaSet.
func TestRollbackRevisionIgnoresTheCoveredPodsOwnRevision(t *testing.T) {
	f := newFixture(t)
	_, old, _ := revisionsFixture(f)

	got := rollbackRevision(f.snapshot().Model, []inventory.EntityID{old})

	if got != "5" {
		t.Fatalf("rollback revision = %q, want 5", got)
	}
}

// A first covered pod with no Deployment does not stop the search.
func TestRollbackRevisionTriesLaterCovers(t *testing.T) {
	f := newFixture(t)
	bare, _, newest := revisionsFixture(f)

	got := rollbackRevision(f.snapshot().Model,
		[]inventory.EntityID{bare, newest})

	if got != "5" {
		t.Fatalf("rollback revision = %q, want 5", got)
	}
}
