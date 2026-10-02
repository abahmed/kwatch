package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rolloutFixture gives shop/api an old and a new ReplicaSet with two
// pods each; the new pods crash, and the old ones too when oldFail.
func rolloutFixture(t *testing.T, oldFail bool) *fixture {
	f := newFixture(t)
	oldPods := f.workload("shop", "api", 2)
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-2")
	f.add(rs)
	f.relate(rs, inventory.OwnedBy, deployment)
	f.change(deployment, 1, "spec.template.spec.containers[0].image")
	for _, name := range []string{"api-2-0", "api-2-1"} {
		pod := inventory.CoreID(kube.KindPod, "shop", name)
		f.add(pod)
		f.relate(pod, inventory.OwnedBy, rs)
		f.fail(pod, "CrashLoop", failingH, 2, "")
	}
	if oldFail {
		for _, pod := range oldPods {
			f.fail(pod, "CrashLoop", failingH, 2, "")
		}
	}
	return f
}

func TestScoreRevisionComparesRevisions(t *testing.T) {
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")
	requireWeight(t, scoreOf(t, rolloutFixture(t, false), deployment,
		scoreRevision), RevisionWeight)
	requireWeight(t, scoreOf(t, rolloutFixture(t, true), deployment,
		scoreRevision), -RevisionPenalty)
}
