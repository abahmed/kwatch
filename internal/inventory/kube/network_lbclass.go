package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrLoadBalancerClass is the spec.loadBalancerClass a Service names:
// the one controller class that may provision its load balancer.
const AttrLoadBalancerClass = "loadbalancer.class"

// setLoadBalancerClass records the class a Service asks for, if any.
func setLoadBalancerClass(
	attrs map[string]inventory.Value, svc *corev1.Service,
) {
	if class := svc.Spec.LoadBalancerClass; class != nil && *class != "" {
		attrs[AttrLoadBalancerClass] = inventory.Text(*class)
	}
}
