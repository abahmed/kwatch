package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// archReplacements replace the pods removedNodeCase ran on n1 with pods
// on n2 that crash with "exec format error": their image is not built
// for n2's CPU. It returns the replacements' containers.
func archReplacements(
	f *fixture, pods []inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for i, pod := range pods {
		f.changes[pod] = append(f.changes[pod], inventory.Change{
			Entity: pod, At: t0.Add(time.Minute), Deleted: true})
		replacement := inventory.CoreID(kube.KindPod, "shop",
			pod.Name+"-new")
		container := containerOf(replacement)
		f.add(replacement, container)
		f.relate(container, inventory.PartOf, replacement)
		f.relate(replacement, inventory.OwnedBy, inventory.CoreID(
			kube.KindReplicaSet, "shop", []string{"api", "web"}[i]+"-1"))
		f.relate(replacement, inventory.RunsOn,
			inventory.CoreID(kube.KindNode, "", "n2"))
		f.findings[container] = []detection.Finding{{
			Entity: container, Reason: "CrashLoopBackOff",
			Mode: detection.ModeCrashLoop, Health: failingH,
			Since: t0.Add(3 * time.Minute),
			Evidence: []detection.Evidence{
				{Label: detection.EvidenceError,
					Value: "exec /app: exec format error"},
				{Label: detection.EvidenceArchNode,
					Value: "arm64 node n2"}}}}
		out = append(out, container)
	}
	return out
}

// Pods that ran fine on a node a scale-down removed, and whose
// replacements crash on a node of another CPU, were broken by the
// removal: it took away the only nodes of the CPU their image is built
// for. The removed node outranks the image's own CPU fault.
func TestRemovedNodeOutranksArchForPodsThatRanThere(t *testing.T) {
	f := newFixture(t)
	containers := archReplacements(f, removedNodeCase(f, true))

	c, ok := f.explain().CauseOf(containers[0])

	if !ok || c.Root != inventory.CoreID(kube.KindNode, "", "n1") ||
		c.Row != RuleNodeRemovedArch {
		t.Fatalf("cause = %s %s, want node//n1 %s", c.Root, c.Row,
			RuleNodeRemovedArch)
	}
}

// A pod with no predecessor on a removed node keeps its own CPU fault.
func TestArchFaultStandsWithoutARemovedNode(t *testing.T) {
	f := newFixture(t)
	f.nodes("zone-a", "n1", "n2")
	pod := f.workload("shop", "api", 1,
		inventory.CoreID(kube.KindNode, "", "n2"))[0]
	f.findings[containerOf(pod)] = []detection.Finding{{
		Entity: containerOf(pod), Reason: "CrashLoopBackOff",
		Mode: detection.ModeCrashLoop, Health: failingH,
		Since: t0.Add(3 * time.Minute),
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceError,
				Value: "exec /app: exec format error"},
			{Label: detection.EvidenceArchNode, Value: "arm64 node n2"}}}}

	c, _ := f.explain().CauseOf(containerOf(pod))

	if c.Root.Kind == kube.KindNode {
		t.Fatalf("cause = %s %s, want no node", c.Root, c.Row)
	}
}
