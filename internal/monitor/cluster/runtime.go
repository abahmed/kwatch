package cluster

import (
	"fmt"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	coordinationv1lister "k8s.io/client-go/listers/coordination/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

const (
	nodeLeaseNamespace       = "kube-node-lease"
	defaultNodeLeaseStaleSec = 90
)

// Runtime owns queue processing for cluster resources. Resource-specific
// policy remains in this family so cluster ownership is explicit.
type Runtime struct {
	runtime    config.RuntimeConfig
	sink       monitor.ReconciliationSink
	now        func() time.Time
	allow      func(string) bool
	mu         sync.Mutex
	started    bool
	configured bool

	quotaLister      corev1lister.ResourceQuotaLister
	limitRangeLister corev1lister.LimitRangeLister
	namespaceLister  corev1lister.NamespaceLister
	leaseLister      coordinationv1lister.LeaseLister
}

// ConfigureSources wires cluster caches and namespace scope once.
func (r *Runtime) ConfigureSources(sources Sources) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("cluster sources cannot change after processing starts")
	}
	if r.configured {
		return fmt.Errorf("cluster sources are already configured")
	}
	r.configured = true
	r.quotaLister = sources.ResourceQuotas
	r.limitRangeLister = sources.LimitRanges
	r.namespaceLister = sources.Namespaces
	r.leaseLister = sources.Leases
	r.allow = sources.NamespaceAllowed
	return nil
}

// beginProcessing closes the source configuration window. Cluster sources
// are wired before the controller starts queue workers.
func (r *Runtime) beginProcessing() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
}

// ProcessResourceQuota evaluates one ResourceQuota queue key.
func (r *Runtime) ProcessResourceQuota(key string, deleted bool) error {
	r.beginProcessing()
	sources := r.sourcesSnapshot()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resourcequota key %q: %w", key, err)
	}
	subject := model.NewObjectRef("resourcequota", namespace, name)
	if deleted || sources.ResourceQuotas == nil {
		r.reconcileGoneWhenDeleted(subject, deleted)
		return nil
	}
	quota, err := sources.ResourceQuotas.ResourceQuotas(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		r.reconcileGone(subject)
		return nil
	}
	if err != nil {
		return fmt.Errorf("get resourcequota %s from cache: %w", key, err)
	}
	r.reconcile(subject, DetectResourceQuotaIssue(quota))
	return nil
}

// ProcessLimitRange evaluates one LimitRange queue key.
func (r *Runtime) ProcessLimitRange(key string, deleted bool) error {
	r.beginProcessing()
	sources := r.sourcesSnapshot()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid limitrange key %q: %w", key, err)
	}
	subject := model.NewObjectRef("limitrange", namespace, name)
	if deleted || sources.LimitRanges == nil {
		r.reconcileGoneWhenDeleted(subject, deleted)
		return nil
	}
	limitRange, err := sources.LimitRanges.LimitRanges(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		r.reconcileGone(subject)
		return nil
	}
	if err != nil {
		return fmt.Errorf("get limitrange %s from cache: %w", key, err)
	}
	r.reconcile(subject, DetectLimitRangeIssue(limitRange))
	return nil
}

// ProcessNamespace evaluates one Namespace queue key.
func (r *Runtime) ProcessNamespace(key string, deleted bool) error {
	r.beginProcessing()
	sources := r.sourcesSnapshot()
	if sources.NamespaceAllowed != nil && !sources.NamespaceAllowed(key) {
		return nil
	}
	subject := model.NewObjectRef("namespace", key, key)
	if deleted || sources.Namespaces == nil {
		r.reconcileGoneWhenDeleted(subject, deleted)
		return nil
	}
	namespace, err := sources.Namespaces.Get(key)
	if apierrors.IsNotFound(err) {
		r.reconcileGone(subject)
		return nil
	}
	if err != nil {
		return fmt.Errorf("get namespace %s from cache: %w", key, err)
	}
	r.reconcile(subject,
		DetectNamespaceIssue(namespace, r.nowTime(),
			r.sustainedMinutes()),
		DetectPodSecurityLabelIssue(namespace),
	)
	return nil
}

// ProcessLease evaluates a node Lease queue key.
func (r *Runtime) ProcessLease(key string, deleted bool) error {
	r.beginProcessing()
	sources := r.sourcesSnapshot()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid lease key %q: %w", key, err)
	}
	if namespace != nodeLeaseNamespace {
		return nil
	}
	if sources.Leases == nil {
		return nil
	}
	if deleted {
		r.resolveLease(name)
		return nil
	}
	lease, err := sources.Leases.Leases(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		r.resolveLease(name)
		return nil
	}
	if err != nil {
		return fmt.Errorf("get lease %s from cache: %w", key, err)
	}
	if finding := DetectNodeLeaseIssue(lease, r.nowTime(),
		r.leaseStaleSeconds()); finding != nil {
		r.process(finding)
	} else {
		r.resolveLease(name)
	}
	return nil
}

// DetectNodeLeaseIssue reports a Lease that has stopped renewing.
func DetectNodeLeaseIssue(
	lease *coordinationv1.Lease, now time.Time, staleSeconds int,
) *model.Observation {
	if lease == nil || lease.Namespace != nodeLeaseNamespace {
		return nil
	}
	if staleSeconds <= 0 {
		staleSeconds = defaultNodeLeaseStaleSec
	}
	if lease.Spec.RenewTime == nil || now.Sub(lease.Spec.RenewTime.Time) >
		time.Duration(staleSeconds)*time.Second {
		age := "never"
		if lease.Spec.RenewTime != nil {
			age = now.Sub(lease.Spec.RenewTime.Time).
				Round(time.Second).String()
		}
		return observe.NodeNamed(
			lease.Name, constant.ReasonNodeLeaseStale,
		).WithLabels(lease.Labels).WithHint(fmt.Sprintf(
			"node lease %s/%s has not renewed for %s;"+
				" kubelet may be unavailable",
			lease.Namespace, lease.Name, age,
		))
	}
	return nil
}

func (r *Runtime) process(observation *model.Observation) {
	if observation == nil || r.sink == nil {
		return
	}
	r.prepare(observation)
	r.sink.Process(observation)
}

func (r *Runtime) prepare(observation *model.Observation) {
	if observation == nil {
		return
	}
	observation.IncludeEvents = r.runtime.Monitors().IncludeEvents()
	observation.IncludeLogs = r.runtime.Monitors().IncludeLogs()
}

func (r *Runtime) reconcile(
	subject model.ObjectRef, observations ...*model.Observation,
) {
	if r.sink == nil {
		return
	}
	for _, observation := range observations {
		if observation == nil {
			continue
		}
		observation.IncludeEvents = r.runtime.Monitors().IncludeEvents()
		observation.IncludeLogs = r.runtime.Monitors().IncludeLogs()
	}
	r.sink.Reconcile(subject, observations)
}

func (r *Runtime) reconcileGoneWhenDeleted(
	subject model.ObjectRef, deleted bool,
) {
	if deleted {
		r.reconcileGone(subject)
	}
}

func (r *Runtime) reconcileGone(subject model.ObjectRef) {
	if r.sink != nil {
		r.sink.ReconcileGone(subject)
	}
}

func (r *Runtime) resolveLease(name string) {
	if r.sink != nil {
		r.sink.Resolve(
			model.ObjectRef{Kind: "node", Name: name},
			constant.ReasonNodeLeaseStale,
		)
	}
}

func (r *Runtime) nowTime() time.Time {
	return r.now()
}

func (r *Runtime) sourcesSnapshot() Sources {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Sources{
		ResourceQuotas:   r.quotaLister,
		LimitRanges:      r.limitRangeLister,
		Namespaces:       r.namespaceLister,
		Leases:           r.leaseLister,
		NamespaceAllowed: r.allow,
	}
}

func (r *Runtime) sustainedMinutes() int {
	return r.runtime.Monitors().ClusterResource().SustainedMinutes
}

func (r *Runtime) leaseStaleSeconds() int {
	return r.runtime.Monitors().ClusterResource().NodeLeaseStaleSeconds
}
