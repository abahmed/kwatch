package statuswatch

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/k8s"
)

func (m *Monitor) deleteCRD(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		name = key
	}
	m.mu.Lock()
	versions := m.crdVersions[name]
	delete(m.crdVersions, name)
	m.mu.Unlock()
	for version := range versions {
		m.stopVersion(version)
	}
}

func (m *Monitor) processAPIService(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if sig := failureSignal(
		u, "apiservice", u.GetName(), m.conditionRules,
	); sig != nil {
		m.correlator.Process(
			event.Event{
				Resource: sig.Resource, Namespace: sig.Namespace,
				PodName: sig.PodName, Reason: sig.Reason,
				Hint: sig.Hint, Labels: sig.Labels,
			}, sig.Owner, nil,
		)
	} else {
		m.resolve("", u.GetName(), constant.ReasonAPIServiceFailure)
	}
}

func (m *Monitor) resolveAPIService(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err == nil && name != "" {
		m.resolve("", name, constant.ReasonAPIServiceFailure)
	}
}

func (m *Monitor) watchCRD(obj interface{}) {
	crd, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
	versions, _, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
	plural, _, _ := unstructured.NestedString(
		crd.Object, "spec", "names", "plural",
	)
	if group == "" || plural == "" || len(versions) == 0 {
		m.reconcileCRDVersions(crd.GetName(), nil)
		return
	}
	desired := make(map[string]struct{})
	for _, rawVersion := range versions {
		versionSpec, ok := rawVersion.(map[string]interface{})
		if !ok {
			continue
		}
		version, _ := versionSpec["name"].(string)
		served, _ := versionSpec["served"].(bool)
		if version == "" || !served {
			continue
		}
		if _, enabled, _ := unstructured.NestedFieldNoCopy(
			versionSpec, "subresources", "status",
		); !enabled {
			continue
		}
		gvr := schema.GroupVersionResource{
			Group: group, Version: version, Resource: plural,
		}
		desired[gvr.String()] = struct{}{}
		m.watchVersion(gvr)
	}
	m.reconcileCRDVersions(crd.GetName(), desired)
}

func (m *Monitor) reconcileCRDVersions(
	crdName string, desired map[string]struct{},
) {
	m.mu.Lock()
	previous := m.crdVersions[crdName]
	m.crdVersions[crdName] = desired
	m.mu.Unlock()
	for key := range previous {
		if _, keep := desired[key]; keep {
			continue
		}
		m.stopVersion(key)
	}
}

func (m *Monitor) watchVersion(gvr schema.GroupVersionResource) {
	key := gvr.String()
	m.mu.Lock()
	if _, exists := m.factories[key]; exists {
		m.mu.Unlock()
		return
	}
	factory := dynamicinformer.NewDynamicSharedInformerFactory(m.client, m.resync)
	m.mu.Unlock()
	informer := factory.ForResource(gvr).Informer()
	if err := informer.SetTransform(k8s.TrimManagedFields); err != nil {
		klog.ErrorS(
			err, "statuswatch: set CRD cache transform", "resource", gvr,
		)
		return
	}
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.processCR,
		UpdateFunc: func(_, obj interface{}) { m.processCR(obj) },
		DeleteFunc: m.resolveCR,
	})
	if err != nil {
		klog.ErrorS(
			err, "statuswatch: register CRD informer", "resource", key,
		)
		return
	}
	versionCtx, stop := context.WithCancel(m.ctx)
	m.mu.Lock()
	if _, exists := m.factories[key]; exists {
		m.mu.Unlock()
		stop()
		return
	}
	m.factories[key] = factory
	m.stops[key] = stop
	m.mu.Unlock()
	factory.Start(versionCtx.Done())
}

func (m *Monitor) stopVersion(key string) {
	m.mu.Lock()
	stop := m.stops[key]
	delete(m.stops, key)
	delete(m.factories, key)
	m.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func (m *Monitor) processCR(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if m.namespaceAllowed != nil && !m.namespaceAllowed(u.GetNamespace()) {
		return
	}
	m.rebuildGraph(u)
	if sig := failureSignal(
		u, "customresource", resourceOwner(u), m.conditionRules,
	); sig != nil {
		m.correlator.Process(
			event.Event{
				Resource: sig.Resource, Namespace: sig.Namespace,
				PodName: sig.PodName, Reason: sig.Reason,
				Hint: sig.Hint, Labels: sig.Labels,
			}, sig.Owner, nil,
		)
	} else {
		m.resolve(
			u.GetNamespace(), resourceOwner(u),
			constant.ReasonCustomResourceFailure,
		)
	}
}

func (m *Monitor) resolveCR(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil ||
		(m.namespaceAllowed != nil && !m.namespaceAllowed(namespace)) {
		return
	}
	if m.graph != nil {
		m.graph.RemoveNode("customresource", namespace, name)
	}
	m.resolve(
		namespace, resourceOwnerParts(namespace, name),
		constant.ReasonCustomResourceFailure,
	)
}

func (m *Monitor) rebuildGraph(u *unstructured.Unstructured) {
	if m.graph == nil {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(u.GetOwnerReferences()))
	for _, ref := range u.GetOwnerReferences() {
		if ref.Name == "" || ref.Kind == "" {
			continue
		}
		targets = append(targets, kwcontext.EdgeTarget{
			Kind:      strings.ToLower(ref.Kind),
			Namespace: u.GetNamespace(), Name: ref.Name,
			Type: "owned_by",
		})
	}
	for _, rule := range m.graphReferences {
		for _, name := range nestedStringValues(u.Object, rule.path) {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: rule.kind, Namespace: u.GetNamespace(), Name: name,
				Type: "references",
			})
		}
	}
	m.graph.ReplaceOutgoingEdges(
		"customresource", u.GetNamespace(), u.GetName(), targets,
	)
}
