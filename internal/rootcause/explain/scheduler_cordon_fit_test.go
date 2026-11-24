package explain

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cordonWithRoom cordons a node with cpu cores and 8 GiB allocatable.
func cordonWithRoom(f *fixture, node inventory.EntityID, cpu float64) {
	f.attrs(node, map[string]inventory.Value{
		kube.AttrUnschedulable:     inventory.Bool(true),
		kube.AttrCPUAllocatable:    inventory.Number(cpu),
		kube.AttrMemoryAllocatable: inventory.Number(8 << 30),
		kube.AttrTaints: inventory.Text(
			"node.kubernetes.io/unschedulable:NoSchedule"),
	})
}

// A cordoned node is named for an unschedulable pod only when the pod
// would fit it were the cordon lifted: room for its requests, taints it
// tolerates and labels its selector accepts.
func TestCordonedNodeNamedOnlyWhenPodWouldFit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(f *fixture, node, pod inventory.EntityID)
		named bool
	}{
		{"fits", func(*fixture, inventory.EntityID, inventory.EntityID) {},
			true},
		{"no room left", func(f *fixture, node, _ inventory.EntityID) {
			busy := f.workload("shop", "batch", 1, node)[0]
			f.attrs(containerOf(busy), map[string]inventory.Value{
				kube.AttrCPUReq: inventory.Number(1.8)})
		}, false},
		{"untolerated taint", func(f *fixture, node, _ inventory.EntityID) {
			f.attrs(node, map[string]inventory.Value{
				kube.AttrTaints: inventory.Text("gpu=true:NoSchedule")})
		}, false},
		{"selector mismatch", func(f *fixture, _, pod inventory.EntityID) {
			spec, _ := json.Marshal(kube.SchedulingSpec{
				Selector: map[string]string{"pool": "arm"}})
			f.attrs(pod, map[string]inventory.Value{
				kube.AttrSchedulingSpec: inventory.Text(string(spec))})
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			node := f.nodes("zone-a", "n3")[0]
			cordonWithRoom(f, node, 2)
			pod := f.workload("shop", "web", 1)[0]
			f.attrs(containerOf(pod), map[string]inventory.Value{
				kube.AttrCPUReq: inventory.Number(0.5)})
			tc.build(f, node, pod)
			pendingWith(f, pod, "0/3 nodes are available: 1 node(s) "+
				"were unschedulable, 2 Insufficient cpu.")
			c, ok := f.explain().CauseOf(pod)
			named := ok && c.Root == node
			if named != tc.named {
				t.Fatalf("cordoned node named = %v, want %v (cause %v)",
					named, tc.named, c.Root)
			}
		})
	}
}
