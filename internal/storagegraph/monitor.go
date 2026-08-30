package storagegraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

var (
	volumeAttachmentGVR = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "volumeattachments"}
	csiDriverGVR        = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "csidrivers"}
)

type Monitor struct {
	client dynamic.Interface
	graph  *kwcontext.ResourceGraph
	resync time.Duration
}

func New(restConfig *rest.Config, graph *kwcontext.ResourceGraph, resync time.Duration) (*Monitor, error) {
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("storagegraph: create dynamic client: %w", err)
	}
	return &Monitor{client: client, graph: graph, resync: resync}, nil
}

func (m *Monitor) Start(ctx context.Context) error {
	factory := dynamicinformer.NewDynamicSharedInformerFactory(m.client, m.resync)
	for _, watched := range []struct {
		gvr schema.GroupVersionResource
		fn  func(interface{})
	}{
		{volumeAttachmentGVR, m.processVolumeAttachment},
		{csiDriverGVR, m.processCSIDriver},
	} {
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
	m.graph.ReplaceMatchingEdges(func(edge kwcontext.Edge) bool {
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

func (m *Monitor) removeNode(gvr schema.GroupVersionResource, obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
		return
	}
	kind := "csidriver"
	if gvr == volumeAttachmentGVR {
		kind = "volumeattachment"
	}
	m.graph.RemoveNode(kind, u.GetNamespace(), u.GetName())
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
