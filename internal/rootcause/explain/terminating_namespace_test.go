package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// missingAccountCase has two pods that reference a ServiceAccount the
// model does not hold, in a namespace in the given phase.
func missingAccountCase(f *fixture, phase string) []inventory.EntityID {
	pods := f.workload("shop", "api", 2)
	account := inventory.CoreID(kube.KindAccount, "shop", "robot")
	ns := inventory.CoreID(kube.KindNamespace, "", "shop")
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: ns,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase: inventory.Text(phase)}})
	for _, pod := range pods {
		f.relate(pod, inventory.References, account)
		f.fail(pod, "Missing.ServiceAccount", failingH, 2, "")
	}
	return pods
}

// What a namespace held is removed with it: an object missing from a
// terminating namespace, or one deleted from it, is never the cause.
func TestExplainIgnoresObjectsMissingFromATerminatingNamespace(t *testing.T) {
	for phase, blamed := range map[string]bool{
		"Active": true, "Terminating": false,
	} {
		t.Run(phase, func(t *testing.T) {
			f := newFixture(t)
			pods := missingAccountCase(f, phase)

			c, ok := f.explain().CauseOf(pods[0])

			got := ok && c.Root.String() == "serviceaccount/shop/robot"
			if got != blamed {
				t.Fatalf("service account blamed = %v, want %v (cause %v)",
					got, blamed, c.Root)
			}
		})
	}
}
