package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// What kwatch remembers is the current revision while something of it
// is ready: the newest ReplicaSet with ready replicas or a ready pod,
// the newest ControllerRevision with a ready pod. An older revision
// still ready during a rollout, or a current one with nothing ready,
// is not remembered.
func TestServingRevisionNamesTheCurrentRevisionWhileReady(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    func(t *testing.T) (*fixture, inventory.EntityID)
		want string
	}{
		{"newest set ready", func(t *testing.T) (*fixture,
			inventory.EntityID) {
			f, _ := servedFixture(t, 0, "")
			rs := inventory.CoreID(kube.KindReplicaSet, "shop", "web-7")
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: rs, Attributes: map[string]inventory.Value{
					kube.AttrRevision:      inventory.Text("7"),
					kube.AttrReadyReplicas: inventory.Number(1)}})
			return f, rs
		}, "web-7"},
		{"only the older set ready", func(t *testing.T) (*fixture,
			inventory.EntityID) {
			f, _ := servedFixture(t, 2, "ready")
			return f, inventory.CoreID(kube.KindPod, "shop", "web-5-x")
		}, ""},
		{"statefulset pod of the newest revision", func(t *testing.T) (
			*fixture, inventory.EntityID) {
			f, _ := revisionFixture(t, kube.KindStatefulSet, "pg-new",
				"ready")
			return f, inventory.CoreID(kube.KindPod, "db", "pg-0")
		}, "pg-new"},
		{"daemonset pod of the older revision", func(t *testing.T) (
			*fixture, inventory.EntityID) {
			f, _ := revisionFixture(t, kube.KindDaemonSet, "old", "ready")
			return f, inventory.CoreID(kube.KindPod, "db", "pg-0")
		}, ""},
		{"newest revision unready", func(t *testing.T) (*fixture,
			inventory.EntityID) {
			f, w := revisionFixture(t, kube.KindStatefulSet, "pg-new",
				"unready")
			return f, w
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, id := tc.f(t)
			_, got, ok := ServingRevision(f.model, id)
			if ok != (tc.want != "") || got != tc.want {
				t.Fatalf("serving = %q (%v), want %q", got, ok, tc.want)
			}
		})
	}
}

// After a completed rollout the previous ReplicaSet is at zero with no
// pod left, so only kwatch's memory of it serving backs a rollback; a
// memory of another revision, or one past the retention, does not.
func TestPreviousServedRemembersACompletedRollout(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   time.Duration
		want bool
	}{
		{"web-5", -time.Hour, true},
		{"web-7", -time.Hour, false},
		{"web-5", -inventory.ServedRetention - time.Hour, false},
	} {
		f, dep := servedFixture(t, 0, "")
		s := f.snapshot()
		f.model.MarkServed(dep, tc.name, s.Now.Add(tc.at))

		got := s.Record(Cause{Root: dep, Covers: []inventory.EntityID{dep},
			Changes: []inventory.Change{{Entity: dep, At: t0}}})

		if got.RollbackRevision != "5" || got.PreviousServed != tc.want {
			t.Errorf("%s seen %v: revision %q, served %v; want 5, %v",
				tc.name, tc.at, got.RollbackRevision, got.PreviousServed,
				tc.want)
		}
	}
}

// A StatefulSet whose previous revision's pods were all replaced is
// backed by the memory of that ControllerRevision serving.
func TestPreviousServedRemembersAControllerRevision(t *testing.T) {
	f, w := revisionFixture(t, kube.KindStatefulSet, "", "")
	s := f.snapshot()
	if s.previousServed(w) {
		t.Fatal("no pod and no memory: want not served")
	}
	f.model.MarkServed(w, "pg-old", s.Now)
	if !s.previousServed(w) {
		t.Fatal("remembered pg-old serving: want served")
	}
}
