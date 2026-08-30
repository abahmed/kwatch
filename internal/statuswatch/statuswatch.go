package statuswatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
)

var (
	apiServiceGVR = schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"}
	crdGVR        = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
)

// Monitor watches APIService and discovered CRD instances. It only treats
// well-known failure-shaped conditions as incidents; arbitrary informational
// status fields are deliberately ignored to prevent operator noise.
type Monitor struct {
	client           dynamic.Interface
	correlator       *correlation.Engine
	resync           time.Duration
	ctx              context.Context
	namespaceAllowed func(string) bool
	mu               sync.Mutex
	factories        map[string]dynamicinformer.DynamicSharedInformerFactory
	stops            map[string]context.CancelFunc
	crdVersions      map[string]map[string]struct{}
	conditionRules   map[string]map[string]bool
	graph            *kwcontext.ResourceGraph
	graphReferences  []graphReferenceRule
}

type graphReferenceRule struct {
	path []string
	kind string
}

// SetGraph connects generic CRD status monitoring to the shared dependency
// graph. The monitor remains useful without it, which keeps graph support
// optional for callers and for clusters where CRD access is restricted.
func (m *Monitor) SetGraph(graph *kwcontext.ResourceGraph) {
	m.graph = graph
}

// SetNamespaceFilter keeps dynamically discovered namespaced resources aligned
// with the controller's resolved namespace scope. Cluster-scoped resources
// pass an empty namespace and are always allowed.
func (m *Monitor) SetNamespaceFilter(filter func(string) bool) {
	m.namespaceAllowed = filter
}

func New(restConfig *rest.Config, correlator *correlation.Engine, resync time.Duration) (*Monitor, error) {
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("statuswatch: create dynamic client: %w", err)
	}
	return &Monitor{
		client: client, correlator: correlator, resync: resync,
		factories: make(map[string]dynamicinformer.DynamicSharedInformerFactory),
		stops:     make(map[string]context.CancelFunc), crdVersions: make(map[string]map[string]struct{}),
		conditionRules: defaultConditionRules(),
	}, nil
}

func (m *Monitor) SetConditionRules(entries []string) {
	rules := make(map[string]map[string]bool)
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		if rules[parts[0]] == nil {
			rules[parts[0]] = make(map[string]bool)
		}
		rules[parts[0]][parts[1]] = true
	}
	if len(rules) == 0 {
		return
	}
	m.conditionRules = rules
}

func (m *Monitor) SetGraphReferenceRules(entries []string) {
	rules := make([]graphReferenceRule, 0, len(entries))
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		path := make([]string, 0)
		for _, part := range strings.Split(strings.TrimSpace(parts[0]), ".") {
			if part != "" {
				path = append(path, part)
			}
		}
		kind := strings.ToLower(strings.TrimSpace(parts[1]))
		if len(path) > 0 && kind != "" {
			rules = append(rules, graphReferenceRule{path: path, kind: kind})
		}
	}
	m.graphReferences = rules
}

func defaultConditionRules() map[string]map[string]bool {
	return map[string]map[string]bool{
		"Ready": {"False": true, "Unknown": true}, "Available": {"False": true, "Unknown": true},
		"Degraded": {"True": true}, "Progressing": {"False": true},
	}
}

func (m *Monitor) Start(ctx context.Context) error {
	m.ctx = ctx
	factory := dynamicinformer.NewDynamicSharedInformerFactory(m.client, m.resync)
	apiInformer := factory.ForResource(apiServiceGVR).Informer()
	if _, err := apiInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: m.processAPIService, UpdateFunc: func(_, obj interface{}) { m.processAPIService(obj) },
		DeleteFunc: m.resolveAPIService,
	}); err != nil {
		return err
	}
	crdInformer := factory.ForResource(crdGVR).Informer()
	if _, err := crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: m.watchCRD, UpdateFunc: func(_, obj interface{}) { m.watchCRD(obj) },
		DeleteFunc: m.deleteCRD,
	}); err != nil {
		return err
	}
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), apiInformer.HasSynced, crdInformer.HasSynced) {
		return fmt.Errorf("statuswatch: informer sync failed")
	}
	return nil
}

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
	if sig := failureSignal(u, "apiservice", u.GetName(), m.conditionRules); sig != nil {
		m.correlator.Process(event.Event{Resource: sig.Resource, Namespace: sig.Namespace, PodName: sig.PodName, Reason: sig.Reason, Hint: sig.Hint, Labels: sig.Labels}, sig.Owner, nil)
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
	plural, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "plural")
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
		if _, enabled, _ := unstructured.NestedFieldNoCopy(versionSpec, "subresources", "status"); !enabled {
			continue
		}
		gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: plural}
		desired[gvr.String()] = struct{}{}
		m.watchVersion(gvr)
	}
	m.reconcileCRDVersions(crd.GetName(), desired)
}

func (m *Monitor) reconcileCRDVersions(crdName string, desired map[string]struct{}) {
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
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.processCR,
		UpdateFunc: func(_, obj interface{}) { m.processCR(obj) },
		DeleteFunc: m.resolveCR,
	})
	if err != nil {
		klog.ErrorS(err, "statuswatch: register CRD informer", "resource", key)
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
	if sig := failureSignal(u, "customresource", resourceOwner(u), m.conditionRules); sig != nil {
		m.correlator.Process(event.Event{Resource: sig.Resource, Namespace: sig.Namespace, PodName: sig.PodName, Reason: sig.Reason, Hint: sig.Hint, Labels: sig.Labels}, sig.Owner, nil)
	} else {
		m.resolve(u.GetNamespace(), resourceOwner(u), constant.ReasonCustomResourceFailure)
	}
}

func (m *Monitor) resolveCR(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil || (m.namespaceAllowed != nil && !m.namespaceAllowed(namespace)) {
		return
	}
	if m.graph != nil {
		m.graph.RemoveNode("customresource", namespace, name)
	}
	m.resolve(namespace, resourceOwnerParts(namespace, name), constant.ReasonCustomResourceFailure)
}

func resourceOwnerParts(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
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
			Kind: strings.ToLower(ref.Kind), Namespace: u.GetNamespace(), Name: ref.Name, Type: "owned_by",
		})
	}
	for _, rule := range m.graphReferences {
		for _, name := range nestedStringValues(u.Object, rule.path) {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: rule.kind, Namespace: u.GetNamespace(), Name: name, Type: "references",
			})
		}
	}
	m.graph.ReplaceOutgoingEdges("customresource", u.GetNamespace(), u.GetName(), targets)
}

func nestedStringValues(value interface{}, path []string) []string {
	if len(path) == 0 {
		if name, ok := value.(string); ok && name != "" {
			return []string{name}
		}
		return nil
	}
	if items, ok := value.([]interface{}); ok {
		var values []string
		for _, item := range items {
			values = append(values, nestedStringValues(item, path)...)
		}
		return values
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return nestedStringValues(object[path[0]], path[1:])
}

func (m *Monitor) resolve(namespace, owner, reason string) {
	m.correlator.MarkResolved(correlation.BuildKey(namespace, owner, reason, ""))
}

func resourceOwner(u *unstructured.Unstructured) string {
	if u.GetNamespace() == "" {
		return u.GetName()
	}
	return u.GetNamespace() + "/" + u.GetName()
}

func failureSignal(u *unstructured.Unstructured, resource, owner string, rules map[string]map[string]bool) *event.Signal {
	conditions, found, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	if !found {
		return nil
	}
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := condition["type"].(string)
		status, _ := condition["status"].(string)
		reason, _ := condition["reason"].(string)
		message, _ := condition["message"].(string)
		failed := false
		if statuses := rules[typ]; statuses != nil {
			failed = statuses[status]
		}
		if !failed {
			continue
		}
		if reason == "" {
			reason = "condition reported " + status
		}
		hint := typ + "=" + status + ": " + reason
		if message != "" {
			hint += " — " + message
		}
		return &event.Signal{Resource: resource, Namespace: u.GetNamespace(), PodName: u.GetName(), Owner: owner, Reason: reasonFor(resource), Labels: u.GetLabels(), Hint: hint}
	}
	return nil
}

func reasonFor(resource string) string {
	if resource == "apiservice" {
		return constant.ReasonAPIServiceFailure
	}
	return constant.ReasonCustomResourceFailure
}
