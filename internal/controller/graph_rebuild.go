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
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
)

func (c *Controller) rebuildReplicaSet(obj interface{}) {
	if c.graph == nil {
		return
	}
	rs, ok := obj.(*appsv1.ReplicaSet)
	if !ok {
		return
	}
	c.graph.RemoveEdgesFrom("replicaset", rs.Namespace, rs.Name, "owned_by")
	c.addOwnedByEdges("replicaset", rs.Namespace, rs.Name, rs.OwnerReferences)
}

func (c *Controller) rebuildJob(obj interface{}) {
	if c.graph == nil {
		return
	}
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return
	}
	c.graph.RemoveEdgesFrom("job", job.Namespace, job.Name, "owned_by")
	c.addOwnedByEdges("job", job.Namespace, job.Name, job.OwnerReferences)
}

func (c *Controller) rebuildIngress(obj interface{}) {
	if c.graph == nil {
		return
	}
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return
	}
	g := c.graph
	g.RemoveEdgesFrom("ingress", ing.Namespace, ing.Name, graphEdgeRoutesTo)
	add := func(svc *networkingv1.IngressServiceBackend) {
		if svc != nil && svc.Name != "" {
			g.AddEdge("ingress", ing.Namespace, ing.Name, "service", ing.Namespace, svc.Name, graphEdgeRoutesTo)
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
}

func (c *Controller) rebuildHorizontalPodAutoscaler(obj interface{}) {
	if c.graph == nil {
		return
	}
	hpa, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		return
	}
	g := c.graph
	g.RemoveEdgesFrom("horizontalpodautoscaler", hpa.Namespace, hpa.Name, graphEdgeScales)
	ref := hpa.Spec.ScaleTargetRef
	if ref.Name != "" && ref.Kind != "" {
		g.AddEdge("horizontalpodautoscaler", hpa.Namespace, hpa.Name,
			strings.ToLower(ref.Kind), hpa.Namespace, ref.Name, graphEdgeScales)
	}
}

func (c *Controller) rebuildNetworkPolicy(obj interface{}) {
	if c.graph == nil {
		return
	}
	np, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return
	}
	g := c.graph
	g.RemoveEdgesFrom("networkpolicy", np.Namespace, np.Name, graphEdgeAppliesTo)
	c.addSelectorEdges("networkpolicy", np.Namespace, np.Name, &np.Spec.PodSelector, graphEdgeAppliesTo)
}

func (c *Controller) rebuildPodDisruptionBudget(obj interface{}) {
	if c.graph == nil {
		return
	}
	pdb, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		return
	}
	g := c.graph
	g.RemoveEdgesFrom("poddisruptionbudget", pdb.Namespace, pdb.Name, graphEdgeProtects)
	if pdb.Spec.Selector == nil {
		return
	}
	c.addSelectorEdges("poddisruptionbudget", pdb.Namespace, pdb.Name, pdb.Spec.Selector, graphEdgeProtects)
}

// addSelectorEdges links the given resource node to every pod in its namespace
// matched by the label selector, encoding "resource protects/applies to pod".
func (c *Controller) addSelectorEdges(kind, ns, name string, sel *metav1.LabelSelector, edgeType string) {
	if c.podLister == nil {
		return
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		klog.ErrorS(err, "failed to build selector for graph edge", "resource", kind, "namespace", ns, "name", name)
		return
	}
	pods, err := c.podLister.Pods(ns).List(selector)
	if err != nil {
		klog.ErrorS(err, "failed to list pods for graph edge", "resource", kind, "namespace", ns, "name", name)
		return
	}
	for _, p := range pods {
		c.graph.AddEdge(kind, ns, name, "pod", ns, p.Name, edgeType)
	}
}

func (c *Controller) rebuildEndpointSlice(obj interface{}) {
	if c.graph == nil {
		return
	}
	eps, ok := obj.(*discoveryv1.EndpointSlice)
	if !ok {
		return
	}
	g := c.graph
	g.RemoveEdgesFrom("endpointslice", eps.Namespace, eps.Name, graphEdgeBacks)
	g.RemoveEdgesFrom("endpointslice", eps.Namespace, eps.Name, graphEdgeTargets)
	if svc := eps.Labels["kubernetes.io/service-name"]; svc != "" {
		g.AddEdge("endpointslice", eps.Namespace, eps.Name, "service", eps.Namespace, svc, graphEdgeBacks)
	}
	for _, ep := range eps.Endpoints {
		if ref := ep.TargetRef; ref != nil && ref.Kind == "Pod" && ref.Name != "" {
			g.AddEdge("endpointslice", eps.Namespace, eps.Name, "pod", eps.Namespace, ref.Name, graphEdgeTargets)
		}
	}
}

func (c *Controller) rebuildPersistentVolumeClaim(obj interface{}) {
	if c.graph == nil {
		return
	}
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return
	}
	g := c.graph
	g.RemoveEdgesFrom("pvc", pvc.Namespace, pvc.Name, graphEdgeBinds)
	g.RemoveEdgesFrom("pvc", pvc.Namespace, pvc.Name, graphEdgeUsesSC)
	if pvc.Spec.VolumeName != "" {
		g.AddEdge("pvc", pvc.Namespace, pvc.Name, "persistentvolume", "", pvc.Spec.VolumeName, graphEdgeBinds)
	}
	if sc := pvc.Spec.StorageClassName; sc != nil && *sc != "" {
		g.AddEdge("pvc", pvc.Namespace, pvc.Name, "storageclass", "", *sc, graphEdgeUsesSC)
	}
}

// rebuildPersistentVolume links a PV to the node(s) it can be scheduled on via
// node affinity (used by local PVs), so a node failure surfaces the volumes
// affected. The affinity selector is resolved against the node informer cache.
func (c *Controller) rebuildPersistentVolume(obj interface{}) {
	if c.graph == nil {
		return
	}
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		return
	}
	g := c.graph
	g.RemoveEdgesFrom("persistentvolume", "", pv.Name, graphEdgeLocalAt)
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return
	}
	if c.nodeLister == nil {
		return
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		selector, err := selectorFromNodeSelectorTerm(term)
		if err != nil {
			continue
		}
		nodes, err := c.nodeLister.List(selector)
		if err != nil {
			klog.ErrorS(err, "failed to list nodes for PV graph edge", "pv", pv.Name)
			return
		}
		for _, n := range nodes {
			g.AddEdge("persistentvolume", "", pv.Name, "node", "", n.Name, graphEdgeLocalAt)
		}
	}
}

func selectorFromNodeSelectorTerm(term corev1.NodeSelectorTerm) (labels.Selector, error) {
	reqs := make([]labels.Requirement, 0, len(term.MatchExpressions))
	for _, expr := range term.MatchExpressions {
		op, err := toSelectionOperator(expr.Operator)
		if err != nil {
			return nil, err
		}
		r, err := labels.NewRequirement(expr.Key, op, expr.Values)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, *r)
	}
	return labels.NewSelector().Add(reqs...), nil
}

// toSelectionOperator lowercases the corev1 NodeSelectorOperator ("In",
// "Exists", ...) into the selection.Operator vocabulary ("in", "exists", ...).
func toSelectionOperator(op corev1.NodeSelectorOperator) (selection.Operator, error) {
	s := strings.ToLower(string(op))
	switch selection.Operator(s) {
	case selection.In, selection.NotIn, selection.Exists, selection.DoesNotExist, selection.GreaterThan, selection.LessThan:
		return selection.Operator(s), nil
	}
	return "", fmt.Errorf("unsupported node selector operator: %s", op)
}
