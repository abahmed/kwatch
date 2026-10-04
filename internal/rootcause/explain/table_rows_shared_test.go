package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// sharedCase builds n workloads, each with a failing and a healthy
// replica, every pod related to shared, and crash-loops the failing
// replicas spread minutes apart while shared shows nothing wrong. For a
// node the failing replicas run on it; otherwise each workload has its
// own pair of nodes, so the node is not what they share. It returns the
// first failing pod.
func sharedCase(
	shared inventory.EntityID, relation inventory.RelationType, n int,
	spread int,
) func(*fixture) inventory.EntityID {
	return func(f *fixture) inventory.EntityID {
		if relation == inventory.References {
			// A referenced object that exists and is fine; one that is
			// missing would be a cause of its own.
			f.add(shared)
		}
		var first inventory.EntityID
		for i := 0; i < n; i++ {
			suffix := string(rune('a' + i))
			nodes := f.nodes("zone-a", "f"+suffix, "h"+suffix)
			if shared.Kind == kube.KindNode {
				nodes = f.nodes("zone-a", shared.Name, "h"+suffix)
			}
			pods := f.workload("shop", "app"+suffix, 2, nodes...)
			for _, pod := range pods {
				if relation != "" {
					f.relate(pod, relation, shared)
				}
			}
			f.fail(pods[0], detection.ModeCrashLoop, failingH, 2+i*spread, "")
			if i == 0 {
				first = pods[0]
			}
		}
		return first
	}
}

var (
	sharedImage = inventory.CoreID(kube.KindImage, "",
		"registry.example.com/base:9")
	sharedConfig  = inventory.CoreID(kube.KindConfigMap, "shop", "common")
	sharedSecret  = inventory.CoreID(kube.KindSecret, "shop", "common-creds")
	sharedAccount = inventory.CoreID(kube.KindAccount, "shop", "runner")
	sharedNode    = inventory.CoreID(kube.KindNode, "", "n1")
)

var sharedRowCases = []rowCase{
	{row: "shared-node", want: "node//n1",
		build: sharedCase(sharedNode, "", 3, 1)},
	{row: "shared-image", want: sharedImage.String(),
		build: sharedCase(sharedImage, inventory.Pulls, 3, 1)},
	{row: "shared-configmap", want: sharedConfig.String(),
		build: sharedCase(sharedConfig, inventory.References, 3, 1)},
	{row: "shared-secret", want: sharedSecret.String(),
		build: sharedCase(sharedSecret, inventory.References, 3, 1)},
	{row: "shared-account", want: sharedAccount.String(),
		build: sharedCase(sharedAccount, inventory.References, 3, 1)},
}

// Two workloads sharing a healthy node are not a pattern: each is its
// own problem.
func TestSharedFactorNeedsThreeWorkloads(t *testing.T) {
	f := newFixture(t)
	pod := sharedCase(sharedNode, "", 2, 1)(f)

	if c, ok := f.explain().CauseOf(pod); ok && c.Root == sharedNode {
		t.Fatalf("two workloads must not suspect their node: %+v", c)
	}
}

// Failures spread over hours share the node by chance, not by cause.
func TestSharedFactorNeedsFailuresThatBeganTogether(t *testing.T) {
	f := newFixture(t)
	pod := sharedCase(sharedNode, "", 3, 60)(f)

	if c, ok := f.explain().CauseOf(pod); ok && c.Root == sharedNode {
		t.Fatalf("failures an hour apart must not suspect the node: %+v", c)
	}
}

// A shared factor is a suspect, never a confident cause.
func TestSharedFactorIsNeverHighConfidence(t *testing.T) {
	f := newFixture(t)
	pod := sharedCase(sharedNode, "", 3, 1)(f)

	c := requireCause(t, f.explain(), pod, "node//n1")
	if c.Confidence > SharedFactorMaxConfidence {
		t.Fatalf("confidence = %.2f, want at most %.2f", c.Confidence,
			SharedFactorMaxConfidence)
	}
}

// A node that mostly runs healthy pods is not what breaks the three
// that crash on it.
func TestSharedFactorNeedsMostDependentsFailing(t *testing.T) {
	f := newFixture(t)
	pod := sharedCase(sharedNode, "", 3, 1)(f)
	f.workload("shop", "calm", 6, sharedNode)

	if c, ok := f.explain().CauseOf(pod); ok && c.Root == sharedNode {
		t.Fatalf("a mostly healthy node must not be blamed: %+v", c)
	}
}
