package storagegraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/k8s"
)

var (
	volumeAttachmentGVR = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "volumeattachments"}
	csiDriverGVR        = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "csidrivers"}
	volumeSnapshotGVR   = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"}
	snapshotContentGVR  = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotcontents"}
	snapshotClassGVR    = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotclasses"}
)

type Monitor struct {
	client          dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	graph           *kwcontext.ResourceGraph
	resync          time.Duration
	correlator      *correlation.Engine
	allowed         func(string) bool
	namespaces      []string
	watchAll        bool
}

func (m *Monitor) SetCorrelator(correlator *correlation.Engine) { m.correlator = correlator }

// SetNamespaceFilter keeps namespaced snapshot failures and graph edges
// aligned with the controller's configured namespace scope.
func (m *Monitor) SetNamespaceFilter(filter func(string) bool) { m.allowed = filter }

func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
}

func New(restConfig *rest.Config, graph *kwcontext.ResourceGraph, resync time.Duration) (*Monitor, error) {
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("storagegraph: create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("storagegraph: create discovery client: %w", err)
	}
	return &Monitor{
		client: client, discoveryClient: discoveryClient, graph: graph,
		resync: resync, watchAll: true,
	}, nil
}

func (m *Monitor) Start(ctx context.Context) error {
	factories := make([]dynamicinformer.DynamicSharedInformerFactory, 0)
	for _, watched := range []struct {
		gvr        schema.GroupVersionResource
		fn         func(interface{})
		namespaced bool
	}{
		{volumeAttachmentGVR, m.processVolumeAttachment, false},
		{csiDriverGVR, m.processCSIDriver, false},
		{volumeSnapshotGVR, m.processVolumeSnapshot, true},
		{snapshotContentGVR, m.processSnapshotContent, false},
		{snapshotClassGVR, m.processSnapshotClass, false},
	} {
		if !m.resourceAvailable(watched.gvr) {
			continue
		}
		for _, namespace := range m.watchNamespaces(watched.namespaced) {
			factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
				m.client, m.resync, namespace, nil,
			)
			informer := factory.ForResource(watched.gvr).Informer()
			if err := informer.SetTransform(k8s.TrimManagedFields); err != nil {
				return fmt.Errorf(
					"storagegraph: set %s cache transform: %w",
					watched.gvr, err,
				)
			}
			if _, err := informer.AddEventHandler(
				cache.ResourceEventHandlerFuncs{
					AddFunc: watched.fn,
					UpdateFunc: func(_, obj interface{}) {
						watched.fn(obj)
					},
					DeleteFunc: func(obj interface{}) {
						m.removeNode(watched.gvr, obj)
					},
				},
			); err != nil {
				return fmt.Errorf(
					"storagegraph: register %s informer: %w",
					watched.gvr, err,
				)
			}
			factories = append(factories, factory)
		}
	}
	for _, factory := range factories {
		factory.Start(ctx.Done())
	}
	return nil
}

func (m *Monitor) watchNamespaces(namespaced bool) []string {
	if !namespaced || m.watchAll {
		return []string{""}
	}
	return append([]string(nil), m.namespaces...)
}

func (m *Monitor) resourceAvailable(gvr schema.GroupVersionResource) bool {
	if m.discoveryClient == nil {
		return true
	}
	resources, err := m.discoveryClient.ServerResourcesForGroupVersion(
		gvr.GroupVersion().String(),
	)
	if err != nil {
		// Optional APIs may be installed after kwatch starts. Let the
		// informer retry absent APIs, but avoid endless retries for RBAC
		// denials that cannot change without an operator action.
		return !apierrors.IsForbidden(err)
	}
	for _, resource := range resources.APIResources {
		if resource.Name == gvr.Resource {
			return true
		}
	}
	return false
}

func (m *Monitor) processVolumeAttachment(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	name := u.GetName()
	targets := make([]kwcontext.EdgeTarget, 0, 4)
	pv, _, _ := unstructured.NestedString(u.Object, "spec", "source", "persistentVolumeName")
	node, _, _ := unstructured.NestedString(u.Object, "spec", "nodeName")
	driver, _, _ := unstructured.NestedString(u.Object, "spec", "attacher")
	if pv != "" {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "persistentvolume", Name: pv, Type: "attaches"})
	}
	if node != "" {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "node", Name: node, Type: "attached_on"})
	}
	if driver != "" {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "csidriver", Name: driver, Type: "handled_by"})
	}
	if attachError(u) {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "volumeattachment_failure", Name: name, Type: "failure"})
		m.reportFailure(u, constant.ReasonVolumeAttachmentFailure, attachmentErrorHint(u))
	} else {
		m.resolveFailure(u, constant.ReasonVolumeAttachmentFailure)
	}
	if m.graph == nil {
		return
	}
	vaKey := "volumeattachment//" + name
	additions := make([]kwcontext.Edge, 0, len(targets)+1)
	for _, target := range targets {
		additions = append(additions, kwcontext.Edge{From: vaKey, To: targetKey(target), Type: target.Type})
	}
	if pv != "" {
		additions = append(additions, kwcontext.Edge{From: "persistentvolume//" + pv, To: vaKey, Type: "has_attachment"})
	}
	m.graph.ReplaceMatchingEdgesAround(vaKey, func(edge kwcontext.Edge) bool {
		return edge.From == vaKey || (edge.Type == "has_attachment" && edge.To == vaKey)
	}, additions)
}

func (m *Monitor) processCSIDriver(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if snapshotError(u) {
		m.reportFailure(u, constant.ReasonVolumeSnapshotFailure, snapshotErrorHint(u))
	} else {
		m.resolveFailure(u, constant.ReasonVolumeSnapshotFailure)
	}
	if m.graph == nil {
		return
	}
	// The node is intentionally created by pod CSI edges. This handler removes
	// stale relationships on deletion; no synthetic health dependency is added
	// without a driver health signal.
	_ = u.GetName()
}

func (m *Monitor) processVolumeSnapshot(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if m.allowed != nil && u.GetNamespace() != "" && !m.allowed(u.GetNamespace()) {
		return
	}
	if snapshotError(u) {
		m.reportFailure(u, constant.ReasonVolumeSnapshotFailure, snapshotErrorHint(u))
	} else {
		m.resolveFailure(u, constant.ReasonVolumeSnapshotFailure)
	}
	if m.graph == nil {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, 2)
	claim, _, _ := unstructured.NestedString(u.Object, "spec", "source", "persistentVolumeClaimName")
	if claim != "" {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "pvc", Namespace: u.GetNamespace(), Name: claim, Type: "snapshots"})
	}
	if snapshotError(u) {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "volumesnapshot_failure", Namespace: u.GetNamespace(), Name: u.GetName(), Type: "failure"})
	}
	m.graph.ReplaceOutgoingEdges("volumesnapshot", u.GetNamespace(), u.GetName(), targets)
}

func (m *Monitor) processSnapshotContent(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, 3)
	ref, _, _ := unstructured.NestedString(u.Object, "spec", "volumeSnapshotRef", "name")
	refNamespace, _, _ := unstructured.NestedString(u.Object, "spec", "volumeSnapshotRef", "namespace")
	if refNamespace == "" {
		refNamespace = u.GetNamespace()
	}
	if ref != "" {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "volumesnapshot", Namespace: refNamespace, Name: ref, Type: "contains"})
	}
	if pv, _, _ := unstructured.NestedString(u.Object, "spec", "source", "volumeHandle"); pv != "" {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "volumehandle", Name: pv, Type: "backs"})
	}
	if snapshotError(u) {
		targets = append(targets, kwcontext.EdgeTarget{Kind: "volumesnapshot_failure", Name: u.GetName(), Type: "failure"})
	}
	m.graph.ReplaceOutgoingEdges("volumesnapshotcontent", "", u.GetName(), targets)
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
	m.graph.ReplaceOutgoingEdges("volumesnapshotclass", "", u.GetName(), []kwcontext.EdgeTarget{{Kind: "csidriver", Name: driver, Type: "uses_csi"}})
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
	if kind == "csidriver" || kind == "volumesnapshotclass" || kind == "volumesnapshotcontent" {
		ns = ""
	}
	m.graph.RemoveNode(kind, ns, name)
}

func targetKey(target kwcontext.EdgeTarget) string {
	return target.Kind + "/" + target.Namespace + "/" + target.Name
}

func attachError(u *unstructured.Unstructured) bool {
	if message, found, _ := unstructured.NestedString(u.Object, "status", "attachError", "message"); found && strings.TrimSpace(message) != "" {
		return true
	}
	if reason, found, _ := unstructured.NestedString(u.Object, "status", "attachError", "reason"); found && strings.TrimSpace(reason) != "" {
		return true
	}
	return false
}

func snapshotError(u *unstructured.Unstructured) bool {
	message, found, _ := unstructured.NestedString(u.Object, "status", "error", "message")
	return found && strings.TrimSpace(message) != ""
}

func attachmentErrorHint(u *unstructured.Unstructured) string {
	reason, _, _ := unstructured.NestedString(u.Object, "status", "attachError", "reason")
	message, _, _ := unstructured.NestedString(u.Object, "status", "attachError", "message")
	return strings.TrimSpace(reason + ": " + message)
}

func snapshotErrorHint(u *unstructured.Unstructured) string {
	message, _, _ := unstructured.NestedString(u.Object, "status", "error", "message")
	return strings.TrimSpace(message)
}

func (m *Monitor) reportFailure(u *unstructured.Unstructured, reason, hint string) {
	if m.correlator == nil {
		return
	}
	if m.allowed != nil && u.GetNamespace() != "" && !m.allowed(u.GetNamespace()) {
		return
	}
	owner := u.GetName()
	if u.GetNamespace() != "" {
		owner = u.GetNamespace() + "/" + owner
	}
	m.correlator.Process(event.Event{Resource: strings.ToLower(u.GetKind()), Namespace: u.GetNamespace(), PodName: u.GetName(), Reason: reason, Hint: hint, Labels: u.GetLabels(), Severity: "high"}, owner, nil)
}

func (m *Monitor) resolveFailure(u *unstructured.Unstructured, reason string) {
	if m.correlator == nil {
		return
	}
	owner := u.GetName()
	if u.GetNamespace() != "" {
		owner = u.GetNamespace() + "/" + owner
	}
	m.correlator.MarkResolved(correlation.BuildKey(u.GetNamespace(), owner, reason, ""))
}
