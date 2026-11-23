package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The scheduler counts every cordoned node it saw. Two cordoned nodes
// against a verdict that counts one means the verdict predates a cordon:
// no node is named as the cause.
func TestCordonCountBelowCordonedNodesNamesNone(t *testing.T) {
	for _, tc := range []struct {
		name    string
		verdict string
		want    string
	}{
		{"counts both", "0/3 nodes are available: 2 node(s) were " +
			"unschedulable, 1 Insufficient cpu.", "node//n3"},
		{"counts one", "0/3 nodes are available: 1 node(s) were " +
			"unschedulable, 2 Insufficient cpu.", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			for _, node := range f.nodes("zone-a", "n3", "n4") {
				f.attrs(node, map[string]inventory.Value{
					kube.AttrUnschedulable: inventory.Bool(true)})
			}
			pod := f.workload("shop", "web", 1)[0]
			pendingWith(f, pod, tc.verdict)
			c, ok := f.explain().CauseOf(pod)
			got := ""
			if ok && c.Root.Kind == kube.KindNode {
				got = c.Root.String()
			}
			if tc.want == "" && got != "" {
				t.Fatalf("named %s, want no node", got)
			}
			if tc.want != "" && got == "" {
				t.Fatalf("named no node, want a cordoned one")
			}
		})
	}
}
