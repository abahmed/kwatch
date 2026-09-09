package statuswatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/correlation"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
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
	namespaces        []string
	watchAll          bool
	mu                sync.Mutex
	factories         map[string]dynamicinformer.DynamicSharedInformerFactory
	stops             map[string]context.CancelFunc
	crdVersions       map[string]map[string]struct{}
	conditionRules    map[string]map[string]bool
	graph             *kwcontext.ResourceGraph
	graphReferences   []graphReferenceRule
	admissionPolicies map[string]struct{}
	admissionBindings map[string]*unstructured.Unstructured
	// serviceLister answers the Service lookup a legacy Endpoints object
	// needs, from the informer cache instead of a live API read.
	serviceLister corev1lister.ServiceLister
	now           func() time.Time
}

// SetServiceLister wires the controller's Service cache. Legacy Endpoints
// objects can only be judged against the Service they back, and there is one
// of those per Endpoints object on every resync; reading the cache turns that
// from an API round trip into a map lookup.
func (m *Monitor) SetServiceLister(lister corev1lister.ServiceLister) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceLister = lister
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
		watchAll: true,
	}, nil
}

func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
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

// cacheSyncTimeout bounds the initial informer sync, matching the
// controller's own bound. A var so tests can shorten it.
var cacheSyncTimeout = 5 * time.Minute

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
	m.startStaticStatusInformers(factory, false)
	if m.watchAll {
		m.startStaticStatusInformers(factory, true)
	}
	factory.Start(ctx.Done())
	if !m.watchAll {
		for _, namespace := range m.namespaces {
			namespacedFactory :=
				dynamicinformer.NewFilteredDynamicSharedInformerFactory(
					m.client, m.resync, namespace, nil,
				)
			m.startStaticStatusInformers(namespacedFactory, true)
			namespacedFactory.Start(ctx.Done())
		}
	}
	// Bounded, because ctx.Done() alone never fires for a cluster where the
	// CRD API is slow or unreachable: the monitor then blocked here forever
	// and its component was reported neither healthy nor failed, just
	// missing. A timeout turns that into an error the caller records.
	syncCtx, cancel := context.WithTimeout(ctx, cacheSyncTimeout)
	defer cancel()
	if !cache.WaitForCacheSync(
		syncCtx.Done(), apiInformer.HasSynced, crdInformer.HasSynced,
	) {
		return fmt.Errorf("statuswatch: informer sync failed")
	}
	return nil
}

// startStaticStatusInformers covers built-in APIs that are not represented by
// the typed controller pipelines but expose durable status conditions. Missing
// APIs (older clusters or disabled feature gates) simply produce no objects.
func (m *Monitor) startStaticStatusInformers(
	factory dynamicinformer.DynamicSharedInformerFactory,
	namespaced bool,
) {
	for _, watched := range staticStatusWatches {
		watched := watched
		if watched.namespaced != namespaced {
			continue
		}
		if !m.resourceAvailable(watched.gvr) {
			continue
		}
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
	return m.resourceAvailable(schema.GroupVersionResource{
		Group: "discovery.k8s.io", Version: "v1",
		Resource: "endpointslices",
	})
}

func (m *Monitor) resourceAvailable(gvr schema.GroupVersionResource) bool {
	if m.discoveryClient == nil {
		return true
	}
	resources, err := m.discoveryClient.ServerResourcesForGroupVersion(
		gvr.GroupVersion().String(),
	)
	if err != nil {
		// A missing or forbidden API must not create a reflector that will
		// retry forever. Transient discovery failures should start the
		// informer so its normal backoff can recover without a restart.
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) ||
			apierrors.IsUnauthorized(err) || apierrors.IsMethodNotSupported(err) {
			return false
		}
		return true
	}
	for _, resource := range resources.APIResources {
		if resource.Name == gvr.Resource {
			return true
		}
	}
	return false
}

func (m *Monitor) processStatic(obj interface{}, watched staticWatch) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || (m.namespaceAllowed != nil && !m.namespaceAllowed(u.GetNamespace())) {
		return
	}
	var sig *model.Observation
	evaluated := true
	switch watched.resource {
	case "endpoints":
		sig, evaluated = m.legacyEndpointSignal(u)
	case "certificatesigningrequest", "podcertificaterequest":
		sig = certificateSignal(u, watched.resource, m.nowTime())
	default:
		sig = failureSignal(u, watched.resource, watched.rules)
	}
	if !evaluated {
		return
	}
	if sig != nil {
		m.correlator.Process(sig)
	} else {
		m.resolveObject(u, watched.resource)
	}
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
	m.correlator.Resolve(
		model.NewObjectRef(watched.resource, namespace, name),
		reasonFor(watched.resource),
	)
}

// resolveObject records that a watched object no longer reports a failing
// condition.
func (m *Monitor) resolveObject(
	u *unstructured.Unstructured, resource string,
) {
	m.correlator.Resolve(
		model.NewObjectRef(resource, u.GetNamespace(), u.GetName()),
		reasonFor(resource),
	)
}

// resolve records that one reason no longer holds for a watched object.
func (m *Monitor) resolve(resource, namespace, name, reason string) {
	m.correlator.Resolve(
		model.NewObjectRef(resource, namespace, name), reason,
	)
}
