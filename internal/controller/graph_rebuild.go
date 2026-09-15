package controller

import (
	"fmt"

	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func (b *graphBuilder) rebuildNetworkPolicy(obj interface{}) {
	if b.graph == nil {
		return
	}
	networkPolicy, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return
	}
	if err := b.rebuildNetworkPolicyChecked(networkPolicy); err != nil {
		klog.ErrorS(err,
			"failed to rebuild networkpolicy graph edges; keeping previous edges",
			"namespace", networkPolicy.Namespace, "name", networkPolicy.Name)
	}
}

func (b *graphBuilder) rebuildNetworkPolicyChecked(
	networkPolicy *networkingv1.NetworkPolicy,
) error {
	return b.replaceSelectorEdges(
		"networkpolicy", networkPolicy.Namespace, networkPolicy.Name,
		&networkPolicy.Spec.PodSelector, graphEdgeAppliesTo,
	)
}

func (b *graphBuilder) rebuildPodDisruptionBudget(obj interface{}) {
	if b.graph == nil {
		return
	}
	budget, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		return
	}
	if err := b.rebuildPodDisruptionBudgetChecked(budget); err != nil {
		klog.ErrorS(err,
			"failed to rebuild poddisruptionbudget graph edges; keeping previous edges",
			"namespace", budget.Namespace, "name", budget.Name)
	}
}

func (b *graphBuilder) rebuildPodDisruptionBudgetChecked(
	budget *policyv1.PodDisruptionBudget,
) error {
	if budget.Spec.Selector == nil {
		b.graph.ReplaceOutgoingEdges(
			"poddisruptionbudget", budget.Namespace, budget.Name, nil,
		)
		return nil
	}
	return b.replaceSelectorEdges(
		"poddisruptionbudget", budget.Namespace, budget.Name,
		budget.Spec.Selector, graphEdgeProtects,
	)
}

func (b *graphBuilder) replaceSelectorEdges(
	kind, namespace, name string,
	selector *metav1.LabelSelector,
	edgeType string,
) error {
	podNames, err := b.selectedPodNames(namespace, selector)
	if err != nil {
		return err
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(podNames))
	for _, podName := range podNames {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "pod", Namespace: namespace, Name: podName, Type: edgeType,
		})
	}
	b.graph.ReplaceOutgoingEdges(kind, namespace, name, targets)
	return nil
}

func (b *graphBuilder) selectedPodNames(
	namespace string, selector *metav1.LabelSelector,
) ([]string, error) {
	if b.podLister == nil {
		return nil, nil
	}
	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("build pod selector: %w", err)
	}
	pods, err := b.podLister.Pods(namespace).List(parsed)
	if err != nil {
		return nil, fmt.Errorf("list selected pods: %w", err)
	}
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names, nil
}

func (b *graphBuilder) rebuildEndpointSlice(obj interface{}) {
	if b.graph == nil {
		return
	}
	eps, ok := obj.(*discoveryv1.EndpointSlice)
	if !ok {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(eps.Endpoints)+1)
	if service := eps.Labels["kubernetes.io/service-name"]; service != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "service", Namespace: eps.Namespace, Name: service,
			Type: graphEdgeBacks,
		})
	}
	for _, endpoint := range eps.Endpoints {
		if !endpointCanReceiveTraffic(endpoint) {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "endpoint", Namespace: eps.Namespace,
				Name: eps.Name + "#" + endpointAddress(endpoint),
				Type: graphEdgeUnready,
			})
		}
		if ref := endpoint.TargetRef; ref != nil &&
			ref.Kind == "Pod" && ref.Name != "" {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "pod", Namespace: eps.Namespace,
				Name: ref.Name, Type: graphEdgeTargets,
			})
		}
	}
	b.graph.ReplaceOutgoingEdges(
		"endpointslice", eps.Namespace, eps.Name, targets,
	)
	if service := eps.Labels["kubernetes.io/service-name"]; service != "" &&
		b.serviceLister != nil {
		if svc, err := b.serviceLister.Services(eps.Namespace).
			Get(service); err == nil {
			if err := b.rebuildServiceChecked(svc); err != nil {
				klog.ErrorS(err,
					"failed to refresh service graph edges after endpointslice change",
					"namespace", eps.Namespace, "service", service)
			}
		}
	}
}
