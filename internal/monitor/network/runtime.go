package network

import (
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
)

const serviceSustain = time.Duration(
	DefaultServiceSustainedSeconds,
) * time.Second

const degradedServiceSustain = 5 * time.Minute

// Runtime owns queue processing for Service, Ingress, and NetworkPolicy.
// Detection remains in the pure functions in this package.
type Runtime struct {
	runtime config.RuntimeConfig
	sink    monitor.ReconciliationSink
	now     func() time.Time

	mu             sync.Mutex
	services       corev1lister.ServiceLister
	pods           corev1lister.PodLister
	nodes          corev1lister.NodeLister
	endpointSlice  discoveryv1lister.EndpointSliceLister
	ingresses      networkingv1lister.IngressLister
	networkPolicy  networkingv1lister.NetworkPolicyLister
	requeueService func(string, time.Duration)
	firstSeen      map[string]time.Time
	started        bool
	configured     bool
}

// ConfigureSources wires all network sources once.
func (r *Runtime) ConfigureSources(sources Sources) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("network sources cannot change after processing starts")
	}
	if r.configured {
		return fmt.Errorf("network sources are already configured")
	}
	r.configured = true
	r.services = sources.Services
	r.pods = sources.Pods
	r.nodes = sources.Nodes
	r.endpointSlice = sources.EndpointSlice
	r.ingresses = sources.Ingresses
	r.networkPolicy = sources.NetworkPolicy
	r.requeueService = sources.RequeueService
	return nil
}

// beginProcessing closes the source configuration window. Informer sources
// are wired before workers begin processing queue items.
func (r *Runtime) beginProcessing() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
}

// ProcessService evaluates one Service queue key.
func (r *Runtime) ProcessService(key string, deleted bool) error {
	r.beginProcessing()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid service key %q: %w", key, err)
	}
	r.mu.Lock()
	lister := r.services
	endpointLister := r.endpointSlice
	podLister := r.pods
	r.mu.Unlock()
	if lister == nil || endpointLister == nil || podLister == nil {
		return nil
	}
	if deleted {
		r.clearService(namespace, name)
		r.reconcileGone(model.NewObjectRef("service", namespace, name))
		return nil
	}
	svc, err := lister.Services(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.clearService(namespace, name)
			r.reconcileGone(model.NewObjectRef("service", namespace, name))
			return nil
		}
		return fmt.Errorf("get service %s/%s from cache: %w", namespace, name, err)
	}
	return r.ProcessServiceObject(svc, false)
}

// ProcessServiceObject evaluates a Service and its cached EndpointSlices.
func (r *Runtime) ProcessServiceObject(
	svc *corev1.Service, deleted bool,
) error {
	r.beginProcessing()
	if svc == nil {
		return nil
	}
	r.mu.Lock()
	eplist := r.endpointSlice
	podLister := r.pods
	nodeLister := r.nodes
	r.mu.Unlock()
	if eplist == nil || podLister == nil {
		return nil
	}
	subject := model.NewObjectRef("service", svc.Namespace, svc.Name)
	if deleted {
		r.clearService(svc.Namespace, svc.Name)
		r.reconcileGone(subject)
		return nil
	}
	selector := labels.Set{
		"kubernetes.io/service-name": svc.Name,
	}.AsSelector()
	epSlices, err := eplist.EndpointSlices(svc.Namespace).List(selector)
	if err != nil {
		return fmt.Errorf(
			"list EndpointSlices for %s/%s: %w",
			svc.Namespace, svc.Name, err,
		)
	}
	key := svc.Namespace + "/" + svc.Name
	var pods []*corev1.Pod
	if len(svc.Spec.Selector) > 0 {
		pods, err = podLister.Pods(svc.Namespace).List(
			labels.SelectorFromSet(svc.Spec.Selector),
		)
		if err != nil {
			return fmt.Errorf("list backend pods for %s/%s: %w",
				svc.Namespace, svc.Name, err)
		}
	}
	endpointFinding := DetectServiceEndpointIssue(svc, epSlices)
	if !hasLiveBackend(pods) {
		// No pod matches the selector, usually a workload scaled to zero on
		// purpose. Nothing is failing to serve; a workload that should run
		// is reported by its own monitor.
		endpointFinding = nil
	}
	degradedFinding := DetectServiceBackendDegradation(
		svc, epSlices, pods,
	)
	for _, finding := range []*model.Observation{
		endpointFinding, degradedFinding,
	} {
		enrichServiceEndpointFinding(finding, pods, nodeLister)
	}
	endpointFinding = r.sustainServiceFinding(
		key, "outage", endpointFinding, serviceSustain,
	)
	degradedFinding = r.sustainServiceFinding(
		key, "degraded", degradedFinding, degradedServiceSustain,
	)
	// EndpointSlices are rewritten during rollouts and briefly publish a
	// different port set; only a mismatch that outlasts that is reported.
	portFinding := r.sustainServiceFinding(
		key, "ports", DetectServicePortIssue(svc, epSlices), serviceSustain,
	)
	r.reconcile(subject, []*model.Observation{
		endpointFinding,
		degradedFinding,
		portFinding,
		DetectServiceStatusIssue(svc, r.nowTime(),
			float64(DefaultServiceSustainedSeconds)),
	})
	return nil
}

// ProcessNetworkPolicy evaluates one NetworkPolicy queue key.
func (r *Runtime) ProcessNetworkPolicy(key string, deleted bool) error {
	r.beginProcessing()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid networkpolicy key %q: %w", key, err)
	}
	subject := model.NewObjectRef("networkpolicy", namespace, name)
	if !r.networkPolicyAvailable() {
		return nil
	}
	if deleted {
		r.reconcileGone(subject)
		return nil
	}
	policy, err := r.networkPolicyForKey(key)
	if err != nil {
		return err
	}
	if policy == nil {
		if !r.networkPolicyAvailable() {
			return nil
		}
		r.reconcileGone(subject)
		return nil
	}
	r.reconcile(subject, []*model.Observation{
		DetectNetworkPolicyIssue(policy),
	})
	return nil
}

// ProcessIngress evaluates one Ingress queue key.
func (r *Runtime) ProcessIngress(key string, deleted bool) error {
	r.beginProcessing()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid ingress key %q: %w", key, err)
	}
	subject := model.NewObjectRef("ingress", namespace, name)
	if !r.ingressListerAvailable() || !r.serviceListerAvailable() {
		return nil
	}
	if deleted {
		r.reconcileGone(subject)
		return nil
	}
	ingress, err := r.ingressForKey(key)
	if err != nil {
		return err
	}
	if ingress == nil {
		if !r.ingressListerAvailable() {
			return nil
		}
		r.reconcileGone(subject)
		return nil
	}
	findings, err := DetectIngressIssueWithLookup(
		ingress,
		r.serviceExists,
	)
	if err != nil {
		return fmt.Errorf(
			"evaluate ingress %s/%s: %w",
			ingress.Namespace, ingress.Name, err,
		)
	}
	r.reconcile(subject, findings)
	return nil
}

func (r *Runtime) serviceListerAvailable() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.services != nil
}

func (r *Runtime) networkPolicyAvailable() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.networkPolicy != nil
}

func (r *Runtime) ingressListerAvailable() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ingresses != nil
}

func (r *Runtime) networkPolicyForKey(
	key string,
) (*networkingv1.NetworkPolicy, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid networkpolicy key %q: %w", key, err)
	}
	r.mu.Lock()
	lister := r.networkPolicy
	r.mu.Unlock()
	if lister == nil {
		return nil, nil
	}
	policy, err := lister.NetworkPolicies(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf(
			"get networkpolicy %s/%s from cache: %w", namespace, name, err,
		)
	}
	return policy, nil
}

func (r *Runtime) ingressForKey(
	key string,
) (*networkingv1.Ingress, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid ingress key %q: %w", key, err)
	}
	r.mu.Lock()
	lister := r.ingresses
	r.mu.Unlock()
	if lister == nil {
		return nil, nil
	}
	ingress, err := lister.Ingresses(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf(
			"get ingress %s/%s from cache: %w", namespace, name, err,
		)
	}
	return ingress, nil
}

func (r *Runtime) serviceExists(namespace, name string) (bool, error) {
	r.mu.Lock()
	lister := r.services
	r.mu.Unlock()
	if lister == nil {
		return false, nil
	}
	_, err := lister.Services(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *Runtime) reconcile(
	subject model.ObjectRef, observations []*model.Observation,
) {
	if r.sink == nil {
		return
	}
	for _, observation := range observations {
		r.prepare(observation)
	}
	r.sink.Reconcile(subject, observations)
}

func (r *Runtime) reconcileGone(subject model.ObjectRef) {
	if r.sink != nil {
		r.sink.ReconcileGone(subject)
	}
}

func (r *Runtime) prepare(observation *model.Observation) {
	if observation == nil {
		return
	}
	observation.IncludeEvents = r.runtime.Monitors().IncludeEvents()
	observation.IncludeLogs = r.runtime.Monitors().IncludeLogs()
}

func (r *Runtime) nowTime() time.Time {
	r.mu.Lock()
	now := r.now
	r.mu.Unlock()
	return now()
}
