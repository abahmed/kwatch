package controller

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1 "k8s.io/api/apps/v1"

	autoscalingv2 "k8s.io/api/autoscaling/v2"

	batchv1 "k8s.io/api/batch/v1"

	discoveryv1 "k8s.io/api/discovery/v1"

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func (c *Controller) rebuildService(obj interface{}) {
	if c.graph == nil {
		return
	}
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	if err := c.rebuildServiceChecked(svc); err != nil {
		klog.ErrorS(err, "failed to rebuild service graph edges; keeping previous edges", "namespace", svc.Namespace, "name", svc.Name)
	}
}

func (c *Controller) rebuildServiceChecked(svc *corev1.Service) error {
	if c.podLister == nil {
		return nil
	}
	if len(svc.Spec.Selector) == 0 {
		c.graph.ReplaceOutgoingEdges("service", svc.Namespace, svc.Name, nil)
		return nil
	}
	pods, err := c.podLister.Pods(svc.Namespace).List(labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return fmt.Errorf("list pods selected by service: %w", err)
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(pods))
	for _, pod := range pods {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "pod", Namespace: svc.Namespace, Name: pod.Name, Type: "selects",
		})
	}
	if c.endpointSliceLister != nil {
		slices, err := c.endpointSliceLister.EndpointSlices(svc.Namespace).List(labels.Everything())
		if err != nil {
			return fmt.Errorf("list endpoint slices for service graph: %w", err)
		}
		for _, eps := range slices {
			if eps.Labels["kubernetes.io/service-name"] == svc.Name {
				targets = append(targets, kwcontext.EdgeTarget{
					Kind: "endpointslice", Namespace: svc.Namespace, Name: eps.Name, Type: graphEdgeProvides,
				})
			}
		}
	}
	c.graph.ReplaceOutgoingEdges("service", svc.Namespace, svc.Name, targets)
	return nil
}

func (c *Controller) rebuildReplicaSet(obj interface{}) {
	if c.graph == nil {
		return
	}
	rs, ok := obj.(*appsv1.ReplicaSet)
	if !ok {
		return
	}
	c.graph.ReplaceOutgoingEdges("replicaset", rs.Namespace, rs.Name, ownedByTargets(rs.Namespace, rs.OwnerReferences))
}

func (c *Controller) rebuildJob(obj interface{}) {
	if c.graph == nil {
		return
	}
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return
	}
	c.graph.ReplaceOutgoingEdges("job", job.Namespace, job.Name, ownedByTargets(job.Namespace, job.OwnerReferences))
}

func (c *Controller) rebuildIngress(obj interface{}) {
	if c.graph == nil {
		return
	}
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0)
	if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "ingressclass", Name: *ing.Spec.IngressClassName, Type: "uses_class",
		})
	}
	add := func(svc *networkingv1.IngressServiceBackend) {
		if svc != nil && svc.Name != "" {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind:      "service",
				Namespace: ing.Namespace,
				Name:      svc.Name,
				Type:      graphEdgeRoutesTo,
			})
		}
	}
	if ing.Spec.DefaultBackend != nil {
		add(ing.Spec.DefaultBackend.Service)
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			add(path.Backend.Service)
		}
	}
	for _, tls := range ing.Spec.TLS {
		if tls.SecretName != "" {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "secret", Namespace: ing.Namespace, Name: tls.SecretName, Type: graphEdgeTLS,
			})
		}
	}
	c.graph.ReplaceOutgoingEdges("ingress", ing.Namespace, ing.Name, targets)
}

func (c *Controller) rebuildHorizontalPodAutoscaler(obj interface{}) {
	if c.graph == nil {
		return
	}
	hpa, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		return
	}
	var targets []kwcontext.EdgeTarget
	ref := hpa.Spec.ScaleTargetRef
	if ref.Name != "" && ref.Kind != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind:      strings.ToLower(ref.Kind),
			Namespace: hpa.Namespace,
			Name:      ref.Name,
			Type:      graphEdgeScales,
		})
	}
	c.graph.ReplaceOutgoingEdges("horizontalpodautoscaler", hpa.Namespace, hpa.Name, targets)
}

func (c *Controller) rebuildNetworkPolicy(obj interface{}) {
	if c.graph == nil {
		return
	}
	np, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return
	}
	if err := c.rebuildNetworkPolicyChecked(np); err != nil {
		klog.ErrorS(err, "failed to rebuild networkpolicy graph edges; keeping previous edges", "namespace", np.Namespace, "name", np.Name)
	}
}

func (c *Controller) rebuildNetworkPolicyChecked(np *networkingv1.NetworkPolicy) error {
	return c.replaceSelectorEdges("networkpolicy", np.Namespace, np.Name, &np.Spec.PodSelector, graphEdgeAppliesTo)
}

func (c *Controller) rebuildPodDisruptionBudget(obj interface{}) {
	if c.graph == nil {
		return
	}
	pdb, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		return
	}
	if err := c.rebuildPodDisruptionBudgetChecked(pdb); err != nil {
		klog.ErrorS(err, "failed to rebuild poddisruptionbudget graph edges; keeping previous edges", "namespace", pdb.Namespace, "name", pdb.Name)
	}
}

func (c *Controller) rebuildPodDisruptionBudgetChecked(pdb *policyv1.PodDisruptionBudget) error {
	if pdb.Spec.Selector == nil {
		c.graph.ReplaceOutgoingEdges("poddisruptionbudget", pdb.Namespace, pdb.Name, nil)
		return nil
	}
	return c.replaceSelectorEdges("poddisruptionbudget", pdb.Namespace, pdb.Name, pdb.Spec.Selector, graphEdgeProtects)
}

func (c *Controller) replaceSelectorEdges(kind, ns, name string, sel *metav1.LabelSelector, edgeType string) error {
	podNames, err := c.selectedPodNames(ns, sel)
	if err != nil {
		return err
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(podNames))
	for _, podName := range podNames {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind:      "pod",
			Namespace: ns,
			Name:      podName,
			Type:      edgeType,
		})
	}
	c.graph.ReplaceOutgoingEdges(kind, ns, name, targets)
	return nil
}

func (c *Controller) selectedPodNames(ns string, sel *metav1.LabelSelector) ([]string, error) {
	if c.podLister == nil {
		return nil, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return nil, fmt.Errorf("build pod selector: %w", err)
	}
	pods, err := c.podLister.Pods(ns).List(selector)
	if err != nil {
		return nil, fmt.Errorf("list selected pods: %w", err)
	}
	names := make([]string, 0, len(pods))
	for _, p := range pods {
		names = append(names, p.Name)
	}
	return names, nil
}

func (c *Controller) rebuildEndpointSlice(obj interface{}) {
	if c.graph == nil {
		return
	}
	eps, ok := obj.(*discoveryv1.EndpointSlice)
	if !ok {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(eps.Endpoints)+1)
	if svc := eps.Labels["kubernetes.io/service-name"]; svc != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind:      "service",
			Namespace: eps.Namespace,
			Name:      svc,
			Type:      graphEdgeBacks,
		})
	}
	for _, ep := range eps.Endpoints {
		if !endpointCanReceiveTraffic(ep) {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: "endpoint", Namespace: eps.Namespace,
				Name: eps.Name + "#" + endpointAddress(ep), Type: graphEdgeUnready,
			})
		}
		if ref := ep.TargetRef; ref != nil && ref.Kind == "Pod" && ref.Name != "" {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind:      "pod",
				Namespace: eps.Namespace,
				Name:      ref.Name,
				Type:      graphEdgeTargets,
			})
		}
	}
	c.graph.ReplaceOutgoingEdges("endpointslice", eps.Namespace, eps.Name, targets)
	if svc := eps.Labels["kubernetes.io/service-name"]; svc != "" && c.serviceLister != nil {
		if obj, err := c.serviceLister.Services(eps.Namespace).Get(svc); err == nil {
			if err := c.rebuildServiceChecked(obj); err != nil {
				klog.ErrorS(err, "failed to refresh service graph edges after endpointslice change", "namespace", eps.Namespace, "service", svc)
			}
		}
	}
}
