package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestScoreQualityOnlyLowersConfidence(t *testing.T) {
	t.Run("unknown health", func(t *testing.T) {
		f := newFixture(t)
		node, pods := nodeWithPods(f, 2, "api")
		f.fail(node, "NotReady", failingH, 1, "")
		f.fail(node, "Heartbeat", detection.Unknown, 1, "")
		for _, pod := range pods {
			f.fail(pod, "NotReady", failingH, 2, "")
		}
		requireWeight(t, scoreOf(t, f, node, scoreQuality),
			-DataQualityPenalty)
	})
	t.Run("missing in an unsynced kind", func(t *testing.T) {
		f := newFixture(t)
		usesCase(f, kube.KindConfigMap, false)
		s := f.snapshot()
		s.Synced = nil
		ref := inventory.CoreID(kube.KindConfigMap, "shop", "db-creds")
		requireWeight(t, scoreSnapshot(t, s, ref, scoreQuality),
			-DataQualityPenalty)
	})
	t.Run("clean data says nothing", func(t *testing.T) {
		f := newFixture(t)
		node, pods := nodeWithPods(f, 2, "api")
		f.fail(node, "NotReady", failingH, 1, "")
		f.fail(pods[0], "NotReady", failingH, 2, "")
		if o := scoreOf(t, f, node, scoreQuality); o.said() {
			t.Fatalf("outcome = %+v, want nothing", o)
		}
	})
}
