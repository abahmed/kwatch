package handler

import (
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
)

func leaseForTest(
	name, namespace string,
	renewed time.Time,
) *coordinationv1.Lease {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if !renewed.IsZero() {
		renewTime := metav1.NewMicroTime(renewed)
		lease.Spec.RenewTime = &renewTime
	}
	return lease
}

func handlerForLeaseTest(
	lease *coordinationv1.Lease,
	now time.Time,
	staleSeconds int,
) (*handler, *correlation.Engine, cache.Indexer) {
	engine := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{
			ClusterResourceMonitor: config.ClusterResourceMonitor{
				Enabled:               true,
				NodeLeaseStaleSeconds: staleSeconds,
			},
		},
		engine,
		testAlertMgr,
	)
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	informer := factory.Coordination().V1().Leases().Informer()
	if lease != nil {
		_ = informer.GetIndexer().Add(lease)
	}
	h.listers.Lease = factory.Coordination().V1().Leases().Lister()
	h.SetClock(func() time.Time { return now })
	return h, engine, informer.GetIndexer()
}

func hasActiveLeaseIncident(snapshot []model.IncidentView) bool {
	for _, incident := range snapshot {
		if incident.Reason == "NodeLeaseStale" &&
			incident.State == model.StateActive {
			return true
		}
	}
	return false
}

func TestDetectNodeLeaseIssueUsesRenewTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stale := leaseForTest(
		"worker-1",
		nodeLeaseNamespace,
		now.Add(-2*time.Minute),
	)
	if sig := DetectNodeLeaseIssue(stale, now, 90); sig == nil {
		t.Fatal("stale node Lease did not produce an observation")
	}
	fresh := leaseForTest(
		"worker-1",
		nodeLeaseNamespace,
		now.Add(-time.Minute),
	)
	if sig := DetectNodeLeaseIssue(fresh, now, 90); sig != nil {
		t.Fatalf("fresh node Lease produced an observation: %v", sig)
	}
}

func TestProcessLeaseStaleThenFreshResolves(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lease := leaseForTest(
		"worker-1",
		nodeLeaseNamespace,
		now.Add(-2*time.Minute),
	)
	h, engine, indexer := handlerForLeaseTest(lease, now, 90)
	if err := h.ProcessLease("kube-node-lease/worker-1", false); err != nil {
		t.Fatal(err)
	}
	if !hasActiveLeaseIncident(engine.Snapshot()) {
		t.Fatal("stale node Lease incident was not created")
	}

	fresh := leaseForTest("worker-1", nodeLeaseNamespace, now)
	if err := indexer.Update(fresh); err != nil {
		t.Fatal(err)
	}
	if err := h.ProcessLease("kube-node-lease/worker-1", false); err != nil {
		t.Fatal(err)
	}
	if hasActiveLeaseIncident(engine.Snapshot()) {
		t.Fatal("fresh node Lease did not resolve the stale incident")
	}
}

func TestProcessLeaseDeletionResolves(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lease := leaseForTest("worker-1", nodeLeaseNamespace, now.Add(-2*time.Minute))
	h, engine, _ := handlerForLeaseTest(lease, now, 90)
	if err := h.ProcessLease("kube-node-lease/worker-1", false); err != nil {
		t.Fatal(err)
	}
	if err := h.ProcessLease("kube-node-lease/worker-1", true); err != nil {
		t.Fatal(err)
	}
	if hasActiveLeaseIncident(engine.Snapshot()) {
		t.Fatal("deleted node Lease did not resolve the stale incident")
	}
}

func TestDetectNodeLeaseIssueIgnoresOtherNamespaces(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lease := leaseForTest("lease", "default", now.Add(-2*time.Minute))
	if sig := DetectNodeLeaseIssue(lease, now, 90); sig != nil {
		t.Fatalf("non-node Lease produced an observation: %v", sig)
	}
}
