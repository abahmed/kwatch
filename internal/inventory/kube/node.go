package kube

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// nodePoolLabels are the well-known labels naming a node's pool, in
// priority order. Kubernetes has no standard key, so providers differ.
var nodePoolLabels = []string{
	"karpenter.sh/nodepool",
	"eks.amazonaws.com/nodegroup",
	"cloud.google.com/gke-nodepool",
	"kubernetes.azure.com/agentpool",
	"agentpool",
	"node.kubernetes.io/pool",
}

// NodeSchema describes Nodes and their topology.
type NodeSchema struct{}

// Kind implements Schema.
func (NodeSchema) Kind() inventory.Kind { return KindNode }

// RelationTypes implements Schema.
func (NodeSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.PartOf}
}

// Describe implements Schema.
func (NodeSchema) Describe(obj any) (Description, bool) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrUnschedulable: inventory.Bool(node.Spec.Unschedulable),
		AttrTaints:        inventory.Text(taintText(node.Spec.Taints)),
		AttrKubelet: inventory.Text(
			node.Status.NodeInfo.KubeletVersion),
		AttrRuntime: inventory.Text(
			node.Status.NodeInfo.ContainerRuntimeVersion),
		AttrKernel: inventory.Text(node.Status.NodeInfo.KernelVersion),
		AttrOS:     inventory.Text(node.Status.NodeInfo.OperatingSystem),
		AttrInstanceType: inventory.Text(
			node.Labels[corev1.LabelInstanceTypeStable]),
	}
	attrs[AttrDeleting] = inventory.Bool(node.DeletionTimestamp != nil)
	if !node.CreationTimestamp.IsZero() {
		attrs[AttrCreated] = inventory.Time(node.CreationTimestamp.Time)
	}
	setMilli(attrs, AttrCPUAllocatable, node.Status.Allocatable.Cpu())
	setQuantity(attrs, AttrMemoryAllocatable,
		node.Status.Allocatable.Memory())
	conditions := make([]condition, 0, len(node.Status.Conditions))
	for _, c := range node.Status.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
		if c.Type == corev1.NodeReady {
			attrs[AttrReady] = inventory.Bool(
				c.Status == corev1.ConditionTrue,
			)
			attrs[AttrReadySince] = inventory.Time(c.LastTransitionTime.Time)
		}
	}
	setConditions(attrs, conditions)
	rel := relations{}
	rel.add(inventory.PartOf,
		inventory.CoreID(KindZone, "",
			node.Labels[corev1.LabelTopologyZone]),
		inventory.CoreID(KindNodePool, "", nodePool(node.Labels)),
	)
	return Description{
		ID: objectID(KindNode, node), UID: string(node.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema.
func (NodeSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.Node)
	after, ok2 := new.(*corev1.Node)
	if !ok1 || !ok2 {
		return nil
	}
	var fields []inventory.FieldChange
	add := func(path, b, a string) {
		if b != a {
			fields = append(fields, inventory.FieldChange{
				Path: path, Before: b, After: a,
			})
		}
	}
	add("spec.unschedulable",
		boolText(before.Spec.Unschedulable),
		boolText(after.Spec.Unschedulable))
	add("spec.taints", taintText(before.Spec.Taints),
		taintText(after.Spec.Taints))
	add("status.nodeInfo.kubeletVersion",
		before.Status.NodeInfo.KubeletVersion,
		after.Status.NodeInfo.KubeletVersion)
	add("status.nodeInfo.containerRuntimeVersion",
		before.Status.NodeInfo.ContainerRuntimeVersion,
		after.Status.NodeInfo.ContainerRuntimeVersion)
	add("status.nodeInfo.kernelVersion",
		before.Status.NodeInfo.KernelVersion,
		after.Status.NodeInfo.KernelVersion)
	return fields
}

func nodePool(labels map[string]string) string {
	for _, key := range nodePoolLabels {
		if value := labels[key]; value != "" {
			return value
		}
	}
	return ""
}

// taintText renders taints in a stable order, ignoring the TimeAdded
// field so re-applied taints are not changes.
func taintText(taints []corev1.Taint) string {
	parts := make([]string, 0, len(taints))
	for _, t := range taints {
		part := t.Key
		if t.Value != "" {
			part += "=" + t.Value
		}
		parts = append(parts, part+":"+string(t.Effect))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
