package scenarios

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// finalizerReasonScenarios are deletions held by a controller kwatch
// cannot see: what its controller wrote in a Warning event is the only
// reason there is.
func finalizerReasonScenarios() []scenario {
	return []scenario{serviceStuckDeletingWarning()}
}

// finalizerOldScenarios are deletions stuck past the old-problem
// horizon, which go quiet.
func finalizerOldScenarios() []scenario {
	return []scenario{serviceStuckDeletingOldQuiet()}
}

// loadBalancerEvent is what a cloud load balancer controller writes when
// it cannot delete the balancer of a Service.
const loadBalancerEvent = "Failed deploy model due to operation error " +
	"Elastic Load Balancing v2: DeleteLoadBalancer, https response " +
	"error StatusCode: 400, RequestID: 1725b5e1, ResourceInUse: Load " +
	"balancer 'arn:aws:elasticloadbalancing:eu-west-1:1:loadbalancer/" +
	"net/k8s-ingress/863dfe' cannot be deleted because it is currently " +
	"associated with another service"

// serviceStuckDeletingWarning: a Service was deleted three days ago,
// well inside the old-problem horizon, and its finalizer is never
// removed. The controller that holds it runs outside the cluster's
// workloads kwatch sees; the Warning events it keeps writing say why.
func serviceStuckDeletingWarning() scenario {
	return scenario{
		expect: expectation{
			Name: "service-stuck-deleting-warning",
			Description: "A Service has been stuck deleting for three " +
				"days; its load balancer controller keeps writing " +
				"a Warning event that says why.",
			Root: "service/kube-addons/ingress-internal", Tier: "digest",
			MaxMessages: 2, Tail: duration(40 * time.Minute),
		},
		build: func(c *cluster) {
			buildStuckService(c, 3*24*time.Hour)
		},
	}
}

// serviceStuckDeletingOldQuiet: the same Service has been stuck
// deleting for six months, past the old-problem horizon. Once the young
// Warning event's finding expires it is an old problem: listed with the
// old problems, never sent.
func serviceStuckDeletingOldQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "service-stuck-deleting-old-quiet",
			Description: "A Service has been stuck deleting for six " +
				"months, past the old-problem horizon: it goes quiet " +
				"though its controller keeps writing a Warning event.",
			Quiet: true, Tail: duration(40 * time.Minute),
		},
		build: func(c *cluster) {
			buildStuckService(c, 180*24*time.Hour)
		},
	}
}

// buildStuckService lists a Service deleted age ago whose finalizer is
// never removed, then the Warning event its load balancer controller
// writes.
func buildStuckService(c *cluster, age time.Duration) {
	c.list(c.node("n1", "zone-a"))
	meta := c.meta("kube-addons", "ingress-internal")
	svc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Service"}}
	svc.SetName(meta.Name)
	svc.SetNamespace(meta.Namespace)
	svc.SetUID(meta.UID)
	svc.SetDeletionTimestamp(timeAt(c, -age))
	svc.SetFinalizers([]string{"service.k8s.aws/resources"})
	c.list(svc)
	c.after(time.Minute)
	c.warn(c.warningEvent(svc, "Service", "FailedDeployModel",
		loadBalancerEvent, "service", 12))
}
