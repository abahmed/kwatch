package storagegraph

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (m *Monitor) processVolumeAttachment(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	name := u.GetName()
	targets := make([]kwcontext.EdgeTarget, 0, 4)
	pv, _, _ := unstructured.NestedString(
		u.Object, "spec", "source", "persistentVolumeName",
	)
	node, _, _ := unstructured.NestedString(u.Object, "spec", "nodeName")
	driver, _, _ := unstructured.NestedString(u.Object, "spec", "attacher")
	if pv != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "persistentvolume", Name: pv, Type: "attaches",
		})
	}
	if node != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "node", Name: node, Type: "attached_on",
		})
	}
	if driver != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "csidriver", Name: driver, Type: "handled_by",
		})
	}
	if attachError(u) {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "volumeattachment_failure", Name: name, Type: "failure",
		})
		m.reportFailure(u, constant.ReasonVolumeAttachmentFailure,
			attachmentErrorHint(u))
	} else {
		m.resolveFailure(u, constant.ReasonVolumeAttachmentFailure)
	}
	if m.graph == nil {
		return
	}
	vaKey := "volumeattachment//" + name
	additions := make([]kwcontext.Edge, 0, len(targets)+1)
	for _, target := range targets {
		additions = append(additions, kwcontext.Edge{
			From: vaKey, To: targetKey(target), Type: target.Type,
		})
	}
	if pv != "" {
		additions = append(additions, kwcontext.Edge{
			From: "persistentvolume//" + pv, To: vaKey,
			Type: "has_attachment",
		})
	}
	m.graph.ReplaceMatchingEdgesAround(vaKey, func(edge kwcontext.Edge) bool {
		return edge.From == vaKey ||
			(edge.Type == "has_attachment" && edge.To == vaKey)
	}, additions)
}

func (m *Monitor) processCSIDriver(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if snapshotError(u) {
		m.reportFailure(u, constant.ReasonVolumeSnapshotFailure,
			snapshotErrorHint(u))
	} else {
		m.resolveFailure(u, constant.ReasonVolumeSnapshotFailure)
	}
}

func (m *Monitor) processVolumeSnapshot(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if m.allowed != nil && u.GetNamespace() != "" &&
		!m.allowed(u.GetNamespace()) {
		return
	}
	if snapshotError(u) {
		m.reportFailure(u, constant.ReasonVolumeSnapshotFailure,
			snapshotErrorHint(u))
	} else {
		m.resolveFailure(u, constant.ReasonVolumeSnapshotFailure)
	}
	if m.graph == nil {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, 2)
	claim, _, _ := unstructured.NestedString(
		u.Object, "spec", "source", "persistentVolumeClaimName",
	)
	if claim != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "pvc", Namespace: u.GetNamespace(), Name: claim,
			Type: "snapshots",
		})
	}
	if snapshotError(u) {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "volumesnapshot_failure", Namespace: u.GetNamespace(),
			Name: u.GetName(), Type: "failure",
		})
	}
	m.graph.ReplaceOutgoingEdges(
		"volumesnapshot", u.GetNamespace(), u.GetName(), targets,
	)
}

func (m *Monitor) processSnapshotContent(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, 3)
	ref, _, _ := unstructured.NestedString(
		u.Object, "spec", "volumeSnapshotRef", "name",
	)
	refNamespace, _, _ := unstructured.NestedString(
		u.Object, "spec", "volumeSnapshotRef", "namespace",
	)
	if refNamespace == "" {
		refNamespace = u.GetNamespace()
	}
	if ref != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "volumesnapshot", Namespace: refNamespace, Name: ref,
			Type: "contains",
		})
	}
	if pv, _, _ := unstructured.NestedString(
		u.Object, "spec", "source", "volumeHandle",
	); pv != "" {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "volumehandle", Name: pv, Type: "backs",
		})
	}
	if snapshotError(u) {
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "volumesnapshot_failure", Name: u.GetName(), Type: "failure",
		})
	}
	m.graph.ReplaceOutgoingEdges(
		"volumesnapshotcontent", "", u.GetName(), targets,
	)
}

func (m *Monitor) processSnapshotClass(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
		return
	}
	driver, _, _ := unstructured.NestedString(u.Object, "driver")
	if driver == "" {
		return
	}
	m.graph.ReplaceOutgoingEdges("volumesnapshotclass", "", u.GetName(),
		[]kwcontext.EdgeTarget{{Kind: "csidriver", Name: driver, Type: "uses_csi"}})
}

func (m *Monitor) removeNode(gvr schema.GroupVersionResource, obj interface{}) {
	if m.graph == nil {
		return
	}
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}
	kind := "csidriver"
	switch gvr {
	case volumeAttachmentGVR:
		kind = "volumeattachment"
	case volumeSnapshotGVR:
		kind = "volumesnapshot"
	case snapshotContentGVR:
		kind = "volumesnapshotcontent"
	case snapshotClassGVR:
		kind = "volumesnapshotclass"
	}
	if kind == "csidriver" || kind == "volumesnapshotclass" ||
		kind == "volumesnapshotcontent" {
		ns = ""
	}
	m.graph.RemoveNode(kind, ns, name)
}

func targetKey(target kwcontext.EdgeTarget) string {
	return model.ObjectKey(target.Kind, target.Namespace, target.Name)
}

func attachError(u *unstructured.Unstructured) bool {
	if message, found, _ := unstructured.NestedString(
		u.Object, "status", "attachError", "message",
	); found && strings.TrimSpace(message) != "" {
		return true
	}
	if reason, found, _ := unstructured.NestedString(
		u.Object, "status", "attachError", "reason",
	); found && strings.TrimSpace(reason) != "" {
		return true
	}
	return false
}

func snapshotError(u *unstructured.Unstructured) bool {
	message, found, _ := unstructured.NestedString(
		u.Object, "status", "error", "message",
	)
	return found && strings.TrimSpace(message) != ""
}

func attachmentErrorHint(u *unstructured.Unstructured) string {
	reason, _, _ := unstructured.NestedString(
		u.Object, "status", "attachError", "reason",
	)
	message, _, _ := unstructured.NestedString(
		u.Object, "status", "attachError", "message",
	)
	return strings.TrimSpace(reason + ": " + message)
}

func snapshotErrorHint(u *unstructured.Unstructured) string {
	message, _, _ := unstructured.NestedString(
		u.Object, "status", "error", "message",
	)
	return strings.TrimSpace(message)
}

func (m *Monitor) reportFailure(
	u *unstructured.Unstructured, reason, hint string,
) {
	if m.incidentSink == nil {
		return
	}
	if m.allowed != nil && u.GetNamespace() != "" &&
		!m.allowed(u.GetNamespace()) {
		return
	}
	obs := observe.ObjectNamed(
		strings.ToLower(u.GetKind()), u.GetNamespace(), u.GetName(), reason,
	).WithLabels(u.GetLabels()).WithSeverity(model.SeverityHigh).WithHint(hint)
	m.incidentSink.Process(obs)
}

func (m *Monitor) resolveFailure(
	u *unstructured.Unstructured, reason string,
) {
	if m.incidentSink == nil {
		return
	}
	m.incidentSink.Resolve(model.NewObjectRef(
		strings.ToLower(u.GetKind()), u.GetNamespace(), u.GetName(),
	), reason)
}
