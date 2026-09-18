package statuswatch

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
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
	scope, _, _ := unstructured.NestedString(crd.Object, "spec", "scope")
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
		for _, namespace := range m.watchNamespaces(scope == "Namespaced") {
			key := versionKey(gvr, namespace)
			desired[key] = struct{}{}
			m.watchVersion(gvr, namespace)
		}
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
		if _, keep := desired[key]; !keep {
			m.stopVersion(key)
		}
	}
}

func (m *Monitor) watchNamespaces(namespaced bool) []string {
	if !namespaced || m.watchAll {
		return []string{""}
	}
	return append([]string(nil), m.namespaces...)
}

func versionKey(gvr schema.GroupVersionResource, namespace string) string {
	return gvr.String() + "|" + namespace
}

func (m *Monitor) watchVersion(
	gvr schema.GroupVersionResource,
	namespace string,
) {
	key := versionKey(gvr, namespace)
	runCtx, generation, ok := m.lifecycleContext()
	if !ok {
		return
	}
	m.mu.Lock()
	if _, exists := m.factories[key]; exists {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	if !m.canWatchVersion(runCtx, gvr, namespace) {
		return
	}
	factory, informer, err := dynamicwatch.NewInformer(
		m.client, m.resync, namespace, gvr, k8s.TrimManagedFields,
	)
	if err != nil {
		klog.ErrorS(err, "statuswatch: create CRD informer", "resource", gvr)
		return
	}
	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.processCR,
		UpdateFunc: func(_, obj interface{}) { m.processCR(obj) },
		DeleteFunc: m.resolveCR,
	})
	if err != nil {
		klog.ErrorS(err, "statuswatch: register CRD informer", "resource", key)
		return
	}
	versionCtx, stop := context.WithCancel(runCtx)
	m.mu.Lock()
	if m.generation != generation || !m.started {
		m.mu.Unlock()
		stop()
		return
	}
	if _, exists := m.factories[key]; exists {
		m.mu.Unlock()
		stop()
		return
	}
	m.factories[key] = factory
	m.stops[key] = stop
	done := make(chan struct{})
	if m.versionDone == nil {
		m.versionDone = make(map[string]chan struct{})
	}
	m.versionDone[key] = done
	runWG := m.runWG
	m.mu.Unlock()
	if runWG == nil {
		stop()
		close(done)
		return
	}
	runWG.Add(1)
	go func() {
		defer runWG.Done()
		defer close(done)
		informer.Run(versionCtx.Done())
	}()
}

func (m *Monitor) canWatchVersion(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	namespace string,
) bool {
	if ctx == nil {
		return false
	}
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := m.client.Resource(gvr).Namespace(namespace).List(
		listCtx, metav1.ListOptions{Limit: 1},
	)
	if err != nil {
		klog.V(2).InfoS(
			"statuswatch: custom resource is not listable; skipping",
			"resource", gvr, "namespace", namespace, "error", err,
		)
		return false
	}
	return true
}

func (m *Monitor) stopVersion(key string) {
	m.mu.Lock()
	stop := m.stops[key]
	done := m.versionDone[key]
	delete(m.stops, key)
	delete(m.factories, key)
	delete(m.versionDone, key)
	m.mu.Unlock()
	if stop != nil {
		stop()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			klog.ErrorS(
				context.DeadlineExceeded,
				"statuswatch: CRD informer shutdown timed out",
				"resource", key,
			)
		}
	}
}
