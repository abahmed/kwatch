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

	kwcontext "github.com/abahmed/kwatch/internal/context"
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
	return &Monitor{client: client, discoveryClient: discoveryClient, graph: graph, resync: resync}, nil
}

func (m *Monitor) Start(ctx context.Context) error {
	factory := dynamicinformer.NewDynamicSharedInformerFactory(m.client, m.resync)
	for _, watched := range []struct {
		gvr schema.GroupVersionResource
		fn  func(interface{})
	}{
		{volumeAttachmentGVR, m.processVolumeAttachment},
		{csiDriverGVR, m.processCSIDriver},
		{volumeSnapshotGVR, m.processVolumeSnapshot},
		{snapshotContentGVR, m.processSnapshotContent},
		{snapshotClassGVR, m.processSnapshotClass},
	} {
		if !m.resourceAvailable(watched.gvr) {
			continue
		}
		informer := factory.ForResource(watched.gvr).Informer()
		if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: watched.fn,
			UpdateFunc: func(_, obj interface{}) {
				watched.fn(obj)
			},
			DeleteFunc: func(obj interface{}) {
				m.removeNode(watched.gvr, obj)
			},
		}); err != nil {
			return fmt.Errorf("storagegraph: register %s informer: %w", watched.gvr, err)
		}
	}
	factory.Start(ctx.Done())
	return nil
}

func (m *Monitor) resourceAvailable(gvr schema.GroupVersionResource) bool {
	if m.discoveryClient == nil {
		return true
	}
	_, err := m.discoveryClient.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err == nil {
		return true
	}
	// Skip APIs that are genuinely absent or forbidden. For transient
	// discovery failures, still create the informer so client-go can retry.
	return !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err)
}

func (m *Monitor) processVolumeAttachment(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
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
	if !ok || m.graph == nil {
		return
	}
	// The node is intentionally created by pod CSI edges. This handler removes
	// stale relationships on deletion; no synthetic health dependency is added
	// without a driver health signal.
	_ = u.GetName()
}

func (m *Monitor) processVolumeSnapshot(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
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
