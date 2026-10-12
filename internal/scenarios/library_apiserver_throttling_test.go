package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// apiThrottlingScenarios are an API server that refuses requests for
// lack of capacity (priority and fairness) while controllers fall behind.
func apiThrottlingScenarios() []scenario {
	return []scenario{apiServerThrottlingStallsControllers()}
}

// apiServerThrottlingStallsControllers: priority and fairness rejects
// twelve requests a second for minutes. Three Deployments are edited and
// their controller, which is healthy but cannot get its requests through,
// never observes the new generation. The API server is the root, not the
// controller manager or the Deployments.
func apiServerThrottlingStallsControllers() scenario {
	return scenario{
		expect: expectation{
			Name: "apiserver-throttling-stalls-controllers",
			Description: "The API server rejects requests with 429 for " +
				"minutes; three edited Deployments are never reconciled " +
				"while the controller manager is healthy.",
			Root: "apiserver//kube-apiserver", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/cart",
				"deployment/shop/search", "deployment/billing/invoices",
				"controller-manager//kube-controller-manager"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := controlPlaneFleet(c)
			c.probe(kube.ControllerManager, "")
			c.after(time.Minute)
			for _, w := range fleet {
				edited := w.deployment.DeepCopy()
				edited.Generation++
				edited.Spec.Template.Annotations = map[string]string{
					"kubectl.kubernetes.io/restartedAt": c.now.UTC().
						Format(time.RFC3339),
				}
				w.deployment = edited
				c.update(edited)
			}
			for range 14 {
				apiLoad(c, map[string]inventory.Value{
					kube.AttrThrottledRate:  inventory.Number(12),
					kube.AttrThrottledLevel: inventory.Text("workload-low"),
					kube.AttrQueuedRequests: inventory.Number(180),
				})
				c.probe(kube.ControllerManager, "")
				c.after(30 * time.Second)
			}
		},
	}
}
