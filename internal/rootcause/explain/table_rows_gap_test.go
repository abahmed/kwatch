package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gapRowCases cover the rows that read a name out of an error: the
// volume a claim is refused, and the group of a failed conversion.
var gapRowCases = []rowCase{
	{row: RuleVolumeBoundElsewhere, want: "persistentvolume//pv-1",
		build: func(f *fixture) inventory.EntityID {
			pv := inventory.CoreID(kube.KindPV, "", "pv-1")
			claim := inventory.CoreID(kube.KindPVC, "shop", "data")
			f.add(pv)
			f.fail(claim, "ClaimFailed", failingH, 2,
				`volume "pv-1" already bound to a different claim.`)
			return claim
		}},
	{row: RuleConversionWebhookDown,
		want: "customresourcedefinition//widgets.acme.example",
		build: func(f *fixture) inventory.EntityID {
			crd := inventory.CoreID(kube.KindCRD, "",
				"widgets.acme.example")
			f.add(crd)
			f.fail(crd, "Webhook.NoEndpoints", failingH, 1,
				"Conversion webhook backend has no ready pods")
			var first inventory.EntityID
			for _, name := range []string{"operator", "sync"} {
				pod := f.workload("acme", name, 1)[0]
				f.fail(containerOf(pod), "CrashLoop", failingH, 2,
					"conversion webhook for acme.example/v1, "+
						"Kind=Widget failed: no endpoints available")
				if first.IsZero() {
					first = containerOf(pod)
				}
			}
			return first
		}},
	{row: RuleSchedulerNames, want: "priorityclass//batch-low",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "api", 1)[0]
			class := inventory.CoreID(kube.KindPriorityClass, "",
				"batch-low")
			f.add(class)
			f.relate(pod, inventory.References, class)
			f.findings[pod] = append(f.findings[pod], detection.Finding{
				Entity: pod, Reason: "Unschedulable",
				Mode: detection.ModePending, Health: failingH,
				Since:   t0.Add(2 * time.Minute),
				Summary: "Pod cannot be scheduled",
				Evidence: []detection.Evidence{{Label: "scheduler",
					Value: "0/3 nodes are available: 3 Insufficient cpu. " +
						"preemption: PreemptionPolicy=Never."}},
			})
			return pod
		}},
}
