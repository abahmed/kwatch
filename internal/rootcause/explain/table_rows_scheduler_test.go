package explain

import (
	"encoding/json"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// pendingWith gives the pod an unschedulable finding with the verdict.
func pendingWith(f *fixture, pod inventory.EntityID, verdict string) {
	f.findings[pod] = append(f.findings[pod], detection.Finding{
		Entity: pod, Reason: "Unschedulable", Mode: detection.ModePending,
		Health: failingH, Since: t0.Add(2 * time.Minute),
		Summary: "Pod cannot be scheduled",
		Evidence: []detection.Evidence{{Label: "scheduler",
			Value: verdict}},
	})
}

// schedulerNamesRowCases cover the kinds of scheduling reason that point
// at an object, one row each.
var schedulerNamesRowCases = []rowCase{
	{row: "scheduler-names-owner", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "api", 1)[0]
			pendingWith(f, pod, "0/2 nodes are available: 2 node has a "+
				"pod using the ReadWriteOncePod PVC.")
			return pod
		}},
	{row: "scheduler-names-claim", want: "persistentvolumeclaim/shop/data",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "db", 1)[0]
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			f.add(claim)
			f.relate(pod, inventory.Mounts, claim)
			pendingWith(f, pod, "0/2 nodes are available: 2 node(s) did "+
				"not have enough free storage.")
			return pod
		}},
	{row: "scheduler-names-pinned-node", want: "node//n4",
		build: func(f *fixture) inventory.EntityID {
			f.nodes("zone-a", "n4")
			pod := f.workload("shop", "agent", 1)[0]
			spec, _ := json.Marshal(kube.SchedulingSpec{
				NodeTerms: [][]kube.Requirement{{{
					Key: kube.FieldPrefix + "metadata.name", Op: "In",
					Values: []string{"n4"}}}}})
			f.attrs(pod, map[string]inventory.Value{
				kube.AttrSchedulingSpec: inventory.Text(string(spec))})
			pendingWith(f, pod, "0/4 nodes are available: 1 "+
				"Insufficient memory, 3 node(s) didn't match Pod's "+
				"node affinity/selector.")
			return pod
		}},
	{row: "scheduler-names-spread", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "api", 1)[0]
			spec, _ := json.Marshal(kube.SchedulingSpec{
				Spread: []kube.Spread{{TopologyKey: "topology.kubernetes.io/zone",
					MaxSkew: 1}}})
			f.attrs(pod, map[string]inventory.Value{
				kube.AttrSchedulingSpec: inventory.Text(string(spec))})
			pendingWith(f, pod, "0/3 nodes are available: 3 node(s) "+
				"didn't match pod topology spread constraints (missing "+
				"required label).")
			return pod
		}},
	{row: "scheduler-names-cordoned-node", want: "node//n3",
		build: func(f *fixture) inventory.EntityID {
			node := f.nodes("zone-a", "n3")[0]
			f.attrs(node, map[string]inventory.Value{
				kube.AttrUnschedulable: inventory.Bool(true)})
			pod := f.workload("shop", "web", 1)[0]
			pendingWith(f, pod, "0/3 nodes are available: 1 node(s) were "+
				"unschedulable, 2 Insufficient cpu.")
			return pod
		}},
	{row: "scheduler-names-scarce-node", want: "node//n2",
		build: func(f *fixture) inventory.EntityID {
			node := f.nodes("zone-a", "n2")[0]
			f.attrs(node, map[string]inventory.Value{
				kube.AttrAllocatable: inventory.Text("cpu=4,hugepages-2Mi=0")})
			pod := f.workload("shop", "db", 1)[0]
			pendingWith(f, pod, "0/2 nodes are available: 2 "+
				"Insufficient hugepages-2Mi.")
			return pod
		}},
	{row: "scheduler-names-affinity-target", want: "deployment/shop/cache",
		build: func(f *fixture) inventory.EntityID {
			f.workload("shop", "cache", 1)
			deploy := inventory.CoreID(kube.KindDeployment, "shop", "cache")
			f.attrs(deploy, map[string]inventory.Value{
				kube.AttrTemplateLabels: inventory.Text("app=cache"),
				kube.AttrReadyReplicas:  inventory.Number(0)})
			pod := f.workload("shop", "api", 1)[0]
			spec, _ := json.Marshal(kube.SchedulingSpec{
				Affinity: []kube.PodTerm{{Selector: []kube.Requirement{
					{Key: "app", Op: "In", Values: []string{"cache"}}},
					TopologyKey: "kubernetes.io/hostname"}}})
			f.attrs(pod, map[string]inventory.Value{
				kube.AttrSchedulingSpec: inventory.Text(string(spec))})
			pendingWith(f, pod, "0/2 nodes are available: 2 node(s) "+
				"didn't match pod affinity rules.")
			return pod
		}},
}
