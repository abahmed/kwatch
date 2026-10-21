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

// loadBalancerEvent is what a cloud load balancer controller writes when
// it cannot delete the balancer of a Service.
const loadBalancerEvent = "Failed deploy model due to operation error " +
	"Elastic Load Balancing v2: DeleteLoadBalancer, https response " +
	"error StatusCode: 400, RequestID: 1725b5e1, ResourceInUse: Load " +
	"balancer 'arn:aws:elasticloadbalancing:eu-west-1:1:loadbalancer/" +
	"net/k8s-ingress/863dfe' cannot be deleted because it is currently " +
	"associated with another service"

// serviceStuckDeletingWarning: a Service was deleted six months ago and
// its finalizer is never removed. The controller that holds it runs
// outside the cluster's workloads kwatch sees; the Warning events it
// keeps writing say why.
func serviceStuckDeletingWarning() scenario {
	return scenario{
		expect: expectation{
			Name: "service-stuck-deleting-warning",
			Description: "A Service has been stuck deleting for six " +
				"months; its load balancer controller keeps writing " +
				"a Warning event that says why.",
			Root: "service/kube-addons/ingress-internal", Tier: "digest",
			MaxMessages: 2, Tail: duration(40 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			meta := c.meta("kube-addons", "ingress-internal")
			svc := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1", "kind": "Service"}}
			svc.SetName(meta.Name)
			svc.SetNamespace(meta.Namespace)
			svc.SetUID(meta.UID)
			svc.SetDeletionTimestamp(timeAt(c, -180*24*time.Hour))
			svc.SetFinalizers([]string{"service.k8s.aws/resources"})
			c.list(svc)
			c.after(time.Minute)
			c.warn(c.warningEvent(svc, "Service", "FailedDeployModel",
				loadBalancerEvent, "service", 12))
		},
	}
}
