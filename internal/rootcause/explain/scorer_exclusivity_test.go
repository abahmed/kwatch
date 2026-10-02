package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestScoreExclusivityComparesSiblingsElsewhere(t *testing.T) {
	t.Run("healthy replicas elsewhere", func(t *testing.T) {
		f := newFixture(t)
		node, pods := nodeWithPods(f, 2, "api")
		f.fail(node, "NotReady", failingH, 1, "")
		for _, pod := range pods {
			f.fail(pod, "NotReady", failingH, 2, "")
		}
		requireWeight(t, scoreOf(t, f, node, scoreExclusivity),
			ExclusivityWeight)
	})
	t.Run("replicas elsewhere fail too", func(t *testing.T) {
		f := newFixture(t)
		node, pods := nodeWithPods(f, 1, "api")
		f.fail(node, "NotReady", failingH, 1, "")
		f.fail(pods[0], "NotReady", failingH, 2, "")
		elsewhere := inventory.CoreID(kube.KindPod, "shop", "api-1-1")
		f.fail(elsewhere, "NotReady", failingH, 2, "")
		requireWeight(t, scoreOf(t, f, node, scoreExclusivity),
			-ExclusivityPenalty)
	})
	t.Run("single zone is vetoed", func(t *testing.T) {
		f := newFixture(t)
		nodes := f.nodes("zone-a", "n1", "n2")
		for _, node := range nodes {
			f.fail(node, "NotReady", failingH, 1, "")
		}
		zone := inventory.CoreID(kube.KindZone, "", "zone-a")
		if o := scoreOf(t, f, zone, scoreExclusivity); o.veto == "" {
			t.Fatalf("outcome = %+v, want a veto", o)
		}
	})
}
