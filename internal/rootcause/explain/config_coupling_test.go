package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withRevisions dates the workload's ReplicaSet at first and adds a
// newer one, owned by the same Deployment, created at second (minutes
// after t0).
func withRevisions(f *fixture, first, second int) {
	rs := func(name string, at time.Time) {
		id := inventory.CoreID(kube.KindReplicaSet, "shop", name)
		f.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
			Attributes: map[string]inventory.Value{
				kube.AttrCreated: inventory.Time(at)}})
		f.relate(id, inventory.OwnedBy, deployment("shop", "api"))
	}
	rs("api-1", t0.Add(time.Duration(first)*time.Minute))
	rs("api-2", t0.Add(time.Duration(second)*time.Minute))
}

// A config edited between two revisions splits the pods exactly as the
// rollout does: the split is no evidence for the config.
func TestConfigVersionYieldsToARolloutWithTheSamePods(t *testing.T) {
	f, cm := configSplit(t, kube.KindConfigMap,
		"configmap/api-config", true)
	withRevisions(f, -60, configChangeMinute+2)

	requireWeight(t, scoreOf(t, f, cm, scoreConfigVersion), 0)
}

// An edit made before the previous revision reached both revisions.
func TestConfigVersionStaysWhenBothRevisionsReadTheEdit(t *testing.T) {
	f, cm := configSplit(t, kube.KindConfigMap,
		"configmap/api-config", true)
	withRevisions(f, configChangeMinute+1, configChangeMinute+2)

	requireWeight(t, scoreOf(t, f, cm, scoreConfigVersion),
		ConfigVersionWeight)
}

// The same pods, seen from the rollout's side, earn it no reward.
func TestRevisionYieldsToAConfigEditBetweenRevisions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		first, edit  int
		wantRewarded bool
	}{
		{"edit between the revisions", -60, configChangeMinute, false},
		{"edit before both revisions", configChangeMinute + 1,
			configChangeMinute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := rolloutFixture(t, false)
			cm := inventory.CoreID(kube.KindConfigMap, "shop", "api-config")
			f.add(cm)
			f.change(cm, tc.edit, "data.DB_HOST")
			withRevisions(f, tc.first, configChangeMinute+2)
			for _, name := range []string{"api-2-0", "api-2-1"} {
				pod := inventory.CoreID(kube.KindPod, "shop", name)
				f.startedPod(pod, configChangeMinute+2,
					"configmap/api-config", false)
			}

			o := scoreOf(t, f, deployment("shop", "api"), scoreRevision)

			if got := o.weight > 0; got != tc.wantRewarded {
				t.Fatalf("outcome = %+v, rewarded = %v, want %v", o, got,
					tc.wantRewarded)
			}
		})
	}
}
