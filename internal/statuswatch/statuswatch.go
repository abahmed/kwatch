package statuswatch

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/k8s"
)

// Monitor watches APIService and discovered CRD instances. It only treats
// well-known failure-shaped conditions as incidents; arbitrary informational
// status fields are deliberately ignored to prevent operator noise.
type Monitor struct {
	client            dynamic.Interface
	discoveryClient   discovery.DiscoveryInterface
	correlator        *correlation.Engine
	resync            time.Duration
	ctx               context.Context
	namespaceAllowed  func(string) bool
	mu                sync.Mutex
	factories         map[string]dynamicinformer.DynamicSharedInformerFactory
	stops             map[string]context.CancelFunc
	crdVersions       map[string]map[string]struct{}
	conditionRules    map[string]map[string]bool
	graph             *kwcontext.ResourceGraph
	graphReferences   []graphReferenceRule
	admissionPolicies map[string]struct{}
	admissionBindings map[string]*unstructured.Unstructured
	now               func() time.Time
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
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("statuswatch: create discovery client: %w", err)
	}
	return &Monitor{
		client: client, discoveryClient: discoveryClient, correlator: correlator, resync: resync,
		factories: make(map[string]dynamicinformer.DynamicSharedInformerFactory),
		stops:     make(map[string]context.CancelFunc), crdVersions: make(map[string]map[string]struct{}),
		conditionRules: defaultConditionRules(), now: time.Now,
		admissionPolicies: make(map[string]struct{}), admissionBindings: make(map[string]*unstructured.Unstructured),
	}, nil
}

// SetClock injects the clock used by time-sensitive status decisions.
func (m *Monitor) SetClock(now func() time.Time) {
	if now != nil {
		m.now = now
	}
}

func (m *Monitor) nowTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return clock.Now()
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

func (m *Monitor) Start(ctx context.Context) error {
	m.ctx = ctx
	factory := dynamicinformer.NewDynamicSharedInformerFactory(m.client, m.resync)
	apiInformer := factory.ForResource(apiServiceGVR).Informer()
	if err := apiInformer.SetTransform(k8s.TrimManagedFields); err != nil {
		return fmt.Errorf("statuswatch: set api service cache transform: %w", err)
	}
	if _, err := apiInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: m.processAPIService, UpdateFunc: func(_, obj interface{}) { m.processAPIService(obj) },
		DeleteFunc: m.resolveAPIService,
	}); err != nil {
		return err
	}
	crdInformer := factory.ForResource(crdGVR).Informer()
	if err := crdInformer.SetTransform(k8s.TrimManagedFields); err != nil {
		return fmt.Errorf("statuswatch: set crd cache transform: %w", err)
	}
	if _, err := crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: m.watchCRD, UpdateFunc: func(_, obj interface{}) { m.watchCRD(obj) },
		DeleteFunc: m.deleteCRD,
	}); err != nil {
		return err
	}
	m.startAdmissionInformers(factory)
	m.startStaticStatusInformers(factory)
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), apiInformer.HasSynced, crdInformer.HasSynced) {
		return fmt.Errorf("statuswatch: informer sync failed")
	}
	return nil
}

// startStaticStatusInformers covers built-in APIs that are not represented by
// the typed controller pipelines but expose durable status conditions. Missing
// APIs (older clusters or disabled feature gates) simply produce no objects.
func (m *Monitor) startStaticStatusInformers(factory dynamicinformer.DynamicSharedInformerFactory) {
	for _, watched := range staticStatusWatches {
		watched := watched
		if watched.resource == "endpoints" && m.endpointSlicesAvailable() {
			continue
		}
		informer := factory.ForResource(watched.gvr).Informer()
		if err := informer.SetTransform(k8s.TrimManagedFields); err != nil {
			klog.ErrorS(err, "statuswatch: set status cache transform", "resource", watched.gvr)
			continue
		}
		if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { m.processStatic(obj, watched) },
			UpdateFunc: func(_, obj interface{}) { m.processStatic(obj, watched) },
			DeleteFunc: func(obj interface{}) { m.resolveStatic(obj, watched) },
		}); err != nil {
			klog.ErrorS(err, "statuswatch: register built-in status informer", "resource", watched.gvr)
		}
	}
}

func (m *Monitor) endpointSlicesAvailable() bool {
	_, err := m.discoveryClient.ServerResourcesForGroupVersion("discovery.k8s.io/v1")
	return err == nil
}

func (m *Monitor) processStatic(obj interface{}, watched staticWatch) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || (m.namespaceAllowed != nil && !m.namespaceAllowed(u.GetNamespace())) {
		return
	}
	owner := resourceOwner(u)
	var sig *event.Signal
	evaluated := true
	switch watched.resource {
	case "endpoints":
		sig, evaluated = m.legacyEndpointSignal(u)
	case "certificatesigningrequest", "podcertificaterequest":
		sig = certificateSignal(u, watched.resource, owner, m.nowTime())
	default:
		sig = failureSignal(u, watched.resource, owner, watched.rules)
	}
	if !evaluated {
		return
	}
	if sig != nil {
		m.correlator.Process(event.Event{Resource: sig.Resource, Namespace: sig.Namespace, PodName: sig.PodName, Reason: sig.Reason, Hint: sig.Hint, Labels: sig.Labels}, sig.Owner, nil)
	} else {
		m.correlator.MarkResolved(correlation.BuildKey(u.GetNamespace(), owner, reasonFor(watched.resource), ""))
	}
}

func (m *Monitor) legacyEndpointSignal(u *unstructured.Unstructured) (*event.Signal, bool) {
	if u.GetNamespace() == "" {
		return nil, true
	}
	service, err := m.client.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}).Namespace(u.GetNamespace()).Get(m.ctx, u.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, false
	}
	selector, _, _ := unstructured.NestedStringMap(service.Object, "spec", "selector")
	clusterIP, _, _ := unstructured.NestedString(service.Object, "spec", "clusterIP")
	typeName, _, _ := unstructured.NestedString(service.Object, "spec", "type")
	if len(selector) == 0 || clusterIP == "" || clusterIP == "None" || typeName == "ExternalName" {
		return nil, true
	}
	subsets, _, _ := unstructured.NestedSlice(u.Object, "subsets")
	for _, raw := range subsets {
		subset, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		addresses, _, _ := unstructured.NestedSlice(subset, "addresses")
		if len(addresses) > 0 {
			return nil, true
		}
	}
	key := u.GetNamespace() + "/" + u.GetName()
	return &event.Signal{Resource: "service", Namespace: u.GetNamespace(), PodName: u.GetName(), Owner: key, Reason: constant.ReasonServiceNoEndpoints, Labels: u.GetLabels(), Hint: fmt.Sprintf("legacy Endpoints object %s has no ready addresses", key)}, true
}

func certificateSignal(u *unstructured.Unstructured, resource, owner string, now time.Time) *event.Signal {
	rules := map[string]map[string]bool{"Denied": {"True": true}, "Failed": {"True": true}}
	if sig := failureSignal(u, resource, owner, rules); sig != nil {
		return sig
	}
	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	approved := false
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := condition["type"].(string)
		status, _ := condition["status"].(string)
		if typ == "Approved" && status == "True" {
			approved = true
		}
	}
	creation := u.GetCreationTimestamp()
	if !approved || creation.IsZero() || now.Sub(creation.Time) < 10*time.Minute {
		return nil
	}
	certificateField := "certificate"
	if resource == "podcertificaterequest" {
		certificateField = "certificateChain"
	}
	certificate, _, _ := unstructured.NestedString(u.Object, "status", certificateField)
	if certificate == "" {
		return &event.Signal{Resource: resource, Namespace: u.GetNamespace(), PodName: u.GetName(), Owner: owner, Reason: reasonFor(resource), Labels: u.GetLabels(), Hint: "certificate request was approved but the signer has not issued a certificate after 10 minutes"}
	}
	block, _ := pem.Decode([]byte(certificate))
	if block == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || cert.NotAfter.Sub(now) > 7*24*time.Hour {
		return nil
	}
	return &event.Signal{Resource: resource, Namespace: u.GetNamespace(), PodName: u.GetName(), Owner: owner, Reason: reasonFor(resource), Labels: u.GetLabels(), Hint: fmt.Sprintf("issued certificate expires at %s", cert.NotAfter.UTC().Format(time.RFC3339))}
}

func (m *Monitor) resolveStatic(obj interface{}, watched staticWatch) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		namespace, name = "", key
	}
	if m.namespaceAllowed != nil && !m.namespaceAllowed(namespace) {
		return
	}
	m.correlator.MarkResolved(correlation.BuildKey(namespace, resourceOwnerParts(namespace, name), reasonFor(watched.resource), ""))
}

func (m *Monitor) resolve(namespace, owner, reason string) {
	m.correlator.MarkResolved(correlation.BuildKey(namespace, owner, reason, ""))
}
