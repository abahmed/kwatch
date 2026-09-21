package cluster

import (
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coordinationv1lister "k8s.io/client-go/listers/coordination/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestRuntimeProcessesClusterResourcesAndDeletion(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	deleting := metav1.NewTime(now.Add(-11 * time.Minute))
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "apps"},
	}
	limitRange := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "limits", Namespace: "apps"},
	}
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "apps", DeletionTimestamp: &deleting},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nodeLeaseNamespace,
			Name:      "worker-1",
		},
		Spec: coordinationv1.LeaseSpec{
			RenewTime: ptrTime(metav1.NewMicroTime(now.Add(-2 * time.Minute))),
		},
	}

	quotaIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{})
	limitRangeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{})
	namespaceIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{})
	leaseIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{})
	if err := quotaIndexer.Add(quota); err != nil {
		t.Fatal(err)
	}
	if err := limitRangeIndexer.Add(limitRange); err != nil {
		t.Fatal(err)
	}
	if err := namespaceIndexer.Add(namespace); err != nil {
		t.Fatal(err)
	}
	if err := leaseIndexer.Add(lease); err != nil {
		t.Fatal(err)
	}

	sink := &clusterCoverageSink{}
	runtime := NewRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
		nowFunc(now))
	err := runtime.ConfigureSources(Sources{
		ResourceQuotas: corev1lister.NewResourceQuotaLister(quotaIndexer),
		LimitRanges:    corev1lister.NewLimitRangeLister(limitRangeIndexer),
		Namespaces:     corev1lister.NewNamespaceLister(namespaceIndexer),
		Leases:         coordinationv1lister.NewLeaseLister(leaseIndexer),
		NamespaceAllowed: func(key string) bool {
			return key != "blocked"
		},
	})
	if err != nil {
		t.Fatalf("ConfigureSources() error = %v", err)
	}

	if err := runtime.ProcessResourceQuota("apps/quota", false); err != nil {
		t.Fatalf("ProcessResourceQuota() error = %v", err)
	}
	if err := runtime.ProcessLimitRange("apps/limits", false); err != nil {
		t.Fatalf("ProcessLimitRange() error = %v", err)
	}
	if err := runtime.ProcessNamespace("apps", false); err != nil {
		t.Fatalf("ProcessNamespace() error = %v", err)
	}
	if err := runtime.ProcessLease(
		nodeLeaseNamespace+"/worker-1", false,
	); err != nil {
		t.Fatalf("ProcessLease() error = %v", err)
	}
	if err := runtime.ProcessLease("other/worker-1", false); err != nil {
		t.Fatalf("non-node lease processing failed: %v", err)
	}
	if err := runtime.ProcessNamespace("blocked", false); err != nil {
		t.Fatalf("blocked namespace processing failed: %v", err)
	}

	if err := runtime.ProcessResourceQuota("apps/quota", true); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessLimitRange("apps/limits", true); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessNamespace("apps", true); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessLease(
		nodeLeaseNamespace+"/worker-1", true,
	); err != nil {
		t.Fatal(err)
	}

	if sink.reconciles != 3 {
		t.Fatalf("reconciles = %d, want 3", sink.reconciles)
	}
	if sink.gone != 3 {
		t.Fatalf("gone = %d, want 3", sink.gone)
	}
	if sink.processed != 1 || sink.resolutions != 1 {
		t.Fatalf("processed/resolutions = %d/%d, want 1/1",
			sink.processed, sink.resolutions)
	}
}

func TestRuntimeHandlesUnavailableSourcesAndConfigurationRules(t *testing.T) {
	sink := &clusterCoverageSink{}
	runtime := NewRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
		time.Now)
	if err := runtime.ConfigureSources(Sources{}); err != nil {
		t.Fatalf("ConfigureSources() error = %v", err)
	}
	if err := runtime.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second ConfigureSources() succeeded")
	}
	if err := runtime.ProcessResourceQuota("bad/key/extra", false); err == nil {
		t.Fatal("invalid ResourceQuota key succeeded")
	}
	if err := runtime.ProcessLimitRange("bad/key/extra", false); err == nil {
		t.Fatal("invalid LimitRange key succeeded")
	}
	if err := runtime.ProcessLease("bad/key/extra", false); err == nil {
		t.Fatal("invalid Lease key succeeded")
	}
	if err := runtime.ProcessNamespace("missing", false); err != nil {
		t.Fatal(err)
	}
	if sink.gone != 0 {
		t.Fatal("unavailable source resolved a subject")
	}
	if err := runtime.ConfigureSources(Sources{}); err == nil {
		t.Fatal("configuration changed after processing started")
	}
}

func TestRuntimeReconcilesMissingCachedObjects(t *testing.T) {
	sink := &clusterCoverageSink{}
	quotaIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{})
	limitRangeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{})
	namespaceIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{})
	runtime := NewRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
		time.Now)
	if err := runtime.ConfigureSources(Sources{
		ResourceQuotas: corev1lister.NewResourceQuotaLister(quotaIndexer),
		LimitRanges:    corev1lister.NewLimitRangeLister(limitRangeIndexer),
		Namespaces:     corev1lister.NewNamespaceLister(namespaceIndexer),
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessResourceQuota("apps/missing", false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessLimitRange("apps/missing", false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ProcessNamespace("missing", false); err != nil {
		t.Fatal(err)
	}
	if sink.gone != 3 {
		t.Fatalf("gone = %d, want 3", sink.gone)
	}
}

func nowFunc(now time.Time) func() time.Time {
	return func() time.Time { return now }
}

type clusterCoverageSink struct {
	processed   int
	resolutions int
	reconciles  int
	gone        int
}

func (s *clusterCoverageSink) Process(
	*model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.processed++
	return nil, model.ActionSkip
}

func (s *clusterCoverageSink) Resolve(model.ObjectRef, string) {
	s.resolutions++
}

func (*clusterCoverageSink) ResolveObserved(*model.Observation) {}

func (s *clusterCoverageSink) Reconcile(
	model.ObjectRef, []*model.Observation,
) {
	s.reconciles++
}

func (s *clusterCoverageSink) ReconcileGone(model.ObjectRef) {
	s.gone++
}
