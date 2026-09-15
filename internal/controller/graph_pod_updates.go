package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// rebuildPodGraph refreshes one pod's relationships.
func (c *Controller) rebuildPodGraph(pod *corev1.Pod, selectorsChanged bool) {
	if c.graph == nil {
		return
	}

	next := c.newGraphBuilder(kwcontext.NewResourceGraph())
	if err := next.addPodToGraphChecked(pod); err != nil {
		klog.ErrorS(
			err, "failed to rebuild pod graph edges; keeping previous edges",
			"namespace", pod.Namespace, "pod", pod.Name,
		)
		return
	}

	podKey := "pod/" + pod.Namespace + "/" + pod.Name
	c.graph.ReplaceMatchingEdgesAround(podKey, func(edge kwcontext.Edge) bool {
		return edge.From == podKey || (edge.To == podKey && edge.Type == "selects")
	}, next.graph.Edges())
	if selectorsChanged {
		c.refreshPodSelectorEdges(pod.Namespace)
	}
}

// refreshPodSelectorEdges repairs relationships whose selector points at a
// pod. Rebuilding the pod alone cannot update those outgoing edges.
func (c *Controller) refreshPodSelectorEdges(namespace string) {
	if c.netpolLister != nil {
		policies, err := c.netpolLister.NetworkPolicies(
			namespace,
		).List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to refresh networkpolicy graph edges",
				"namespace", namespace)
		} else {
			for _, policy := range policies {
				if err := c.rebuildNetworkPolicyChecked(policy); err != nil {
					klog.ErrorS(err, "failed to refresh networkpolicy graph edges",
						"namespace", policy.Namespace, "name", policy.Name)
				}
			}
		}
	}
	if c.pdbLister == nil {
		return
	}
	budgets, err := c.pdbLister.PodDisruptionBudgets(
		namespace,
	).List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to refresh poddisruptionbudget graph edges",
			"namespace", namespace)
		return
	}
	for _, budget := range budgets {
		if err := c.rebuildPodDisruptionBudgetChecked(budget); err != nil {
			klog.ErrorS(err, "failed to refresh poddisruptionbudget graph edges",
				"namespace", budget.Namespace, "name", budget.Name)
		}
	}
}
