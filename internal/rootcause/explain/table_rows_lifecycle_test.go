package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var lifecycleRowCases = []rowCase{
	{row: "custom-resource-failing",
		want: "kafkacluster.kafka.example/data/events",
		build: func(f *fixture) inventory.EntityID {
			cr := inventory.NewEntityID("kafka.example", "kafkacluster",
				"data", "events")
			sts := inventory.CoreID(kube.KindStatefulSet, "data", "events")
			pod := inventory.CoreID(kube.KindPod, "data", "events-0")
			f.add(cr, sts, pod)
			f.relate(sts, inventory.OwnedBy, cr)
			f.relate(pod, inventory.OwnedBy, sts)
			f.fail(cr, "Condition.Ready", failingH, 1, "")
			f.fail(pod, "CrashLoop", failingH, 2, "")
			return pod
		}},
	{row: "budget-blocks-drain", want: "poddisruptionbudget/shop/ledger",
		build: func(f *fixture) inventory.EntityID {
			nodes := f.nodes("zone-a", "n1", "n2")
			pod := f.workload("shop", "ledger", 1, nodes[1])[0]
			budget := inventory.CoreID(kube.KindPDB, "shop", "ledger")
			f.add(budget)
			f.links[budget] = []Link{{Type: inventory.Selects, To: pod}}
			f.fail(nodes[1], "Draining", degradedH, 1, "")
			f.fail(budget, "Budget.BlocksDrain", failingH, 1, "")
			return nodes[1]
		}},
	{row: "node-removed", want: "node//n1",
		build: func(f *fixture) inventory.EntityID {
			pods := removedNodeCase(f, false)
			for _, pod := range pods {
				f.fail(pod, "StatusUnknown", failingH, 2, "")
			}
			return pods[0]
		}},
	{row: "node-removed-capacity", want: "node//n1",
		build: func(f *fixture) inventory.EntityID {
			pods := removedNodeCase(f, true)
			var first inventory.EntityID
			for i, pod := range pods {
				replacement := inventory.CoreID(kube.KindPod, "shop",
					pod.Name+"-new")
				f.add(replacement)
				f.relate(replacement, inventory.OwnedBy,
					inventory.CoreID(kube.KindReplicaSet, "shop",
						[]string{"api", "web"}[i]+"-1"))
				f.findings[replacement] = []detection.Finding{{
					Entity: replacement, Mode: "Unschedulable",
					Health: degradedH, Since: t0.Add(3 * time.Minute),
					Evidence: []detection.Evidence{{Label: "scheduler",
						Value: "0/1 nodes are available: 1 Insufficient " +
							"cpu."}},
				}}
				if first.IsZero() {
					first = replacement
				}
			}
			return first
		}},
}

// removedNodeCase runs one pod of api and web on n1, then deletes n1 a
// minute after t0, and its pods too when they were replaced. It returns
// the pods that ran there.
func removedNodeCase(f *fixture, replaced bool) []inventory.EntityID {
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := []inventory.EntityID{
		f.workload("shop", "api", 1, nodes[0])[0],
		f.workload("shop", "web", 1, nodes[0])[0],
	}
	f.model.Apply(inventory.Observation{Kind: inventory.Gone,
		Entity: nodes[0], Source: "test", At: t0.Add(time.Minute)})
	if replaced {
		for _, pod := range pods {
			f.model.Apply(inventory.Observation{Kind: inventory.Gone,
				Entity: pod, Source: "test", At: t0.Add(time.Minute)})
		}
	}
	f.changes[nodes[0]] = append(f.changes[nodes[0]], inventory.Change{
		Entity: nodes[0], At: t0.Add(time.Minute), Deleted: true})
	return pods
}
