package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Advisory states are conditions that could become a failure and are not
// one yet: a quota used exactly, pods throttled but ready, a budget that
// allows no disruption beside nothing that needs one. Like the other
// non-events they may never cause a message, not even a digest, however
// long they last. They are written in the calm namespace so a message
// about them is counted.

// calmQuotaFull: a namespace's pod quota is used up exactly. Every
// workload has its desired size and nothing has been refused.
func calmQuotaFull(c *cluster) {
	nodes := calmNodes(c)
	calmFleet(c, "pool", 6, nodes)
	quota := &corev1.ResourceQuota{
		ObjectMeta: clusterMeta(c, calmPrefix, "pod-cap", "quota"),
	}
	hard := corev1.ResourceList{corev1.ResourcePods: resource.MustParse("6")}
	quota.Spec.Hard, quota.Status.Hard = hard, hard
	quota.Status.Used = corev1.ResourceList{
		corev1.ResourcePods: resource.MustParse("6"),
	}
	c.list(quota)
	c.after(10 * time.Minute)
}

// calmThrottled: containers are CPU-throttled most of the time while
// every pod is ready and nothing restarts.
func calmThrottled(c *cluster) {
	nodes := calmNodes(c)
	w := c.deployment(calmPrefix, "web", "registry.example.com/web:9", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: "/healthz",
					Port: intstr.FromInt32(8080)}},
			PeriodSeconds: 10, FailureThreshold: 3,
		}
		spec.Containers[0].Resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("200m"),
		}
	})
	c.list(w.objects())
	c.list(w.pod(0, nodes[0]), w.pod(1, nodes[1]))
	for range 10 {
		c.after(time.Minute)
		for i := range 2 {
			pod := w.pod(i, nodes[i])
			c.emit(inventory.Observation{
				Kind: inventory.Observed, Source: "cadvisor", At: c.now,
				Entity: kube.ContainerID(pod.Namespace, pod.Name, "app"),
				Attributes: map[string]inventory.Value{
					kube.AttrThrottledPct: inventory.Number(70),
				},
			})
		}
	}
}

// calmBudgetFull: a budget on a single-replica Deployment allows no
// disruption, as it always does; a node that holds none of its pods is
// cordoned. Nothing is stuck.
func calmBudgetFull(c *cluster) {
	nodes := calmNodes(c)
	w := calmFleet(c, "ledger", 1, nodes)
	c.list(calmBudget(c, w))
	c.after(time.Minute)
	cordoned := last(c, c.node(nodes[2], "calm-zone"))
	cordoned.Spec.Unschedulable = true
	cordoned.Spec.Taints = []corev1.Taint{{
		Key: "node.kubernetes.io/unschedulable", Effect: "NoSchedule",
	}}
	c.update(cordoned)
	for range 10 {
		c.after(time.Minute)
		c.update(calmBudget(c, w))
	}
}

// calmBudget is the budget of w that allows no disruption.
func calmBudget(c *cluster, w *workload) *policyv1.PodDisruptionBudget {
	one := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: clusterMeta(c, calmPrefix, "ledger", "pdb"),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: w.deployment.Spec.Template.Labels},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			CurrentHealthy: 1, DesiredHealthy: 1, ExpectedPods: 1,
		},
	}
}
