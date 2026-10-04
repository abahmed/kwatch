package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A faulty node shows in its pods in many ways, so the node is still
// suspected when they fail with different reasons.
func TestSharedNodeMayShowDifferentReasons(t *testing.T) {
	f := newFixture(t)
	modes := []detection.Mode{detection.ModeCrashLoop,
		detection.ModeOOMKilled, detection.ModeProbe}
	var first inventory.EntityID
	for i, mode := range modes {
		suffix := string(rune('a' + i))
		nodes := f.nodes("zone-a", sharedNode.Name, "h"+suffix)
		pods := f.workload("shop", "app"+suffix, 2, nodes...)
		f.fail(pods[0], mode, failingH, 2+i, "")
		if i == 0 {
			first = pods[0]
		}
	}

	requireCause(t, f.explain(), first, "node//n1")
}

// configMapCase fails n workloads on one node. The first fails because
// its ConfigMap is missing; the others crash-loop with nothing to
// explain them but the node. It returns the first workload's container
// and the last workload's pod.
func configMapCase(
	f *fixture, n int,
) (configPod, otherPod inventory.EntityID) {
	for i := 0; i < n; i++ {
		suffix := string(rune('a' + i))
		nodes := f.nodes("zone-a", sharedNode.Name, "h"+suffix)
		pods := f.workload("shop", "app"+suffix, 2, nodes...)
		if i > 0 {
			f.fail(pods[0], detection.ModeCrashLoop, failingH, 2+i, "")
			otherPod = pods[0]
			continue
		}
		missing := inventory.CoreID(kube.KindConfigMap, "shop", "db-creds")
		f.relate(pods[0], inventory.References, missing)
		f.fail(containerOf(pods[0]), "CreateError.Config", failingH, 2,
			"configmap shop/db-creds not found")
		configPod = containerOf(pods[0])
	}
	return configPod, otherPod
}

// A shared node never takes a failure a specific cause explains: the
// failure with the missing ConfigMap stays with the ConfigMap, and the
// node only takes the failures nothing specific explains.
func TestSharedNodeLeavesFailuresASpecificCauseExplains(t *testing.T) {
	f := newFixture(t)
	configPod, otherPod := configMapCase(f, 4)

	got := f.explain()
	requireCause(t, got, otherPod, "node//n1")
	for _, area := range got.Areas {
		for _, cause := range area.Causes {
			if cause.Root.Name == "db-creds" &&
				containsID(cause.Covers, configPod) {
				return
			}
		}
	}
	t.Fatalf("the ConfigMap must keep the failure it explains: %+v", got)
}

// Once a specific cause explains one of three failures, the two left
// no longer look like failures that began together on a shared node.
func TestSharedNodeNeedsThreeFailuresNothingSpecificExplains(t *testing.T) {
	f := newFixture(t)
	_, otherPod := configMapCase(f, 3)

	if c, ok := f.explain().CauseOf(otherPod); ok && c.Root == sharedNode {
		t.Fatalf("two leftover failures must not suspect the node: %+v", c)
	}
}
