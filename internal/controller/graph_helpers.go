package controller

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func endpointCanReceiveTraffic(ep discoveryv1.Endpoint) bool {
	if ep.Conditions.Terminating != nil && *ep.Conditions.Terminating {
		return false
	}
	if ep.Conditions.Ready != nil {
		return *ep.Conditions.Ready
	}
	return ep.Conditions.Serving == nil || *ep.Conditions.Serving
}

func endpointAddress(ep discoveryv1.Endpoint) string {
	if len(ep.Addresses) > 0 && ep.Addresses[0] != "" {
		return ep.Addresses[0]
	}
	return "unknown"
}

func (c *Controller) rebuildPersistentVolumeClaim(obj interface{}) {
	if c.graph == nil {
		return
	}
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return
	}
	var targets []kwcontext.EdgeTarget
	if pvc.Spec.VolumeName != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "persistentvolume", Name: pvc.Spec.VolumeName, Type: graphEdgeBinds,
		})
	}
	if sc := pvc.Spec.StorageClassName; sc != nil && *sc != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "storageclass", Name: *sc, Type: graphEdgeUsesSC,
		})
	}
	c.graph.ReplaceOutgoingEdges("pvc", pvc.Namespace, pvc.Name, targets)
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
	if err := c.rebuildPersistentVolumeChecked(pv); err != nil {
		klog.ErrorS(
			err,
			"failed to rebuild persistentvolume graph edges; keeping previous edges",
			"pv", pv.Name,
		)
	}
}

func (c *Controller) rebuildPersistentVolumeChecked(
	pv *corev1.PersistentVolume,
) error {
	nodeNames, err := c.persistentVolumeNodeNames(pv)
	if err != nil {
		return err
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(nodeNames))
	for _, nodeName := range nodeNames {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "node", Name: nodeName, Type: graphEdgeLocalAt,
		})
	}
	if pv.Spec.StorageClassName != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "storageclass", Name: pv.Spec.StorageClassName, Type: graphEdgeUsesSC,
		})
	}
	c.graph.ReplaceOutgoingEdges("persistentvolume", "", pv.Name, targets)
	return nil
}

func (c *Controller) persistentVolumeNodeNames(
	pv *corev1.PersistentVolume,
) ([]string, error) {
	if pv.Spec.NodeAffinity == nil ||
		pv.Spec.NodeAffinity.Required == nil || c.nodeLister == nil {
		return nil, nil
	}
	var names []string
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		selector, err := selectorFromNodeSelectorTerm(term)
		if err != nil {
			return nil, fmt.Errorf("build node selector: %w", err)
		}
		nodes, err := c.nodeLister.List(selector)
		if err != nil {
			return nil, fmt.Errorf("list matching nodes: %w", err)
		}
		for _, n := range nodes {
			names = append(names, n.Name)
		}
	}
	return names, nil
}

func selectorFromNodeSelectorTerm(
	term corev1.NodeSelectorTerm,
) (labels.Selector, error) {
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
func toSelectionOperator(
	op corev1.NodeSelectorOperator,
) (selection.Operator, error) {
	s := strings.ToLower(string(op))
	switch selection.Operator(s) {
	case selection.In, selection.NotIn, selection.Exists,
		selection.DoesNotExist, selection.GreaterThan, selection.LessThan:
		return selection.Operator(s), nil
	}
	return "", fmt.Errorf("unsupported node selector operator: %s", op)
}
