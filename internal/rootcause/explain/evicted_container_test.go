package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

// A container that exited with code 137 because the kubelet evicted its
// pod is explained by the node that evicted it, like the pod.
func TestEvictedPodsContainerExitFollowsTheNode(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1")
	f.fail(nodes[0], detection.ModeDiskPressure, failingH, 1, "")
	effect := nodes[0]
	for _, name := range []string{"orders", "payments"} {
		pod := f.workload("shop", name, 1, nodes[0])[0]
		f.fail(pod, detection.ModeEvicted, failingH, 2, "ephemeral-storage")
		f.fail(containerOf(pod), "Error", failingH, 2, "exited with 137")
		effect = containerOf(pod)
	}

	c := requireCause(t, f.explain(), effect, nodes[0].String())
	if c.Row != "node-disk-pressure" {
		t.Fatalf("row = %s, want node-disk-pressure", c.Row)
	}
}
