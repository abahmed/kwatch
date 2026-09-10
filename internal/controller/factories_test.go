package controller

import (
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestNewFactoriesMultiNamespaceIncludesClusterScope(t *testing.T) {
	client := fake.NewSimpleClientset()
	scope := namespaceScope{namespaces: []string{"one", "two"}}
	fs, factories := newFactories(client, scope, nil, 0)

	if fs.clusterScoped == nil {
		t.Fatal("multi-namespace scope must include a cluster-scoped factory")
	}
	if len(fs.perNamespace) != 2 {
		t.Fatalf("expected 2 namespace factories, got %d", len(fs.perNamespace))
	}
	if len(factories) != 4 {
		t.Fatalf(
			"expected 4 factories including cluster scope and node leases, got %d",
			len(factories),
		)
	}
	if fs.nodeLeaseFactory == nil {
		t.Fatal("multi-namespace scope must include a node Lease factory")
	}
}

func TestNewFactoriesAlwaysWatchesNodeLeaseNamespace(t *testing.T) {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "worker-1",
			Namespace: nodeLeaseNamespace,
		},
	}
	client := fake.NewSimpleClientset(lease)
	fs, factories := newFactories(
		client,
		namespaceScope{namespaces: []string{"one", "two"}},
		[]string{nodeLeaseNamespace},
		0,
	)
	leaseInformers := fs.leaseInformers()
	stop := make(chan struct{})
	defer close(stop)
	for _, factory := range factories {
		factory.Start(stop)
	}
	if !cache.WaitForCacheSync(stop, leaseInformers[0].HasSynced) {
		t.Fatal("dedicated node Lease informer did not sync")
	}

	if fs.leaseLister() == nil {
		t.Fatal("node Lease lister is nil")
	}
	if _, err := fs.leaseLister().Leases(nodeLeaseNamespace).Get(
		"worker-1",
	); err != nil {
		t.Fatalf("dedicated node Lease informer missed kube-node-lease: %v", err)
	}
}

func TestEnqueueLeaseSweepQueuesAllLeases(t *testing.T) {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "worker-1",
			Namespace: nodeLeaseNamespace,
		},
	}
	factory := newNodeLeaseFactory(fake.NewSimpleClientset(), 0)
	informer := factory.Coordination().V1().Leases().Informer()
	if err := informer.GetIndexer().Add(lease); err != nil {
		t.Fatal(err)
	}
	c := &Controller{
		lease:       newResourcePipeline("lease", "leases"),
		leaseLister: factory.Coordination().V1().Leases().Lister(),
	}
	defer c.lease.shutdown()

	c.enqueueLeaseSweep()
	key, quit := c.lease.queue.Get()
	if quit {
		t.Fatal("Lease queue shut down during sweep")
	}
	defer c.lease.queue.Done(key)
	if key != nodeLeaseNamespace+"/worker-1" {
		t.Fatalf("unexpected Lease sweep key %q", key)
	}
}

func TestLeaseSweepIntervalTracksStaleThreshold(t *testing.T) {
	tests := []struct {
		staleSeconds int
		want         time.Duration
	}{
		{staleSeconds: 0, want: 30 * time.Second},
		{staleSeconds: 1, want: time.Second},
		{staleSeconds: 30, want: 10 * time.Second},
		{staleSeconds: 90, want: 30 * time.Second},
		{staleSeconds: 600, want: 30 * time.Second},
	}
	for _, test := range tests {
		if got := leaseSweepInterval(test.staleSeconds); got != test.want {
			t.Errorf(
				"leaseSweepInterval(%d) = %s, want %s",
				test.staleSeconds,
				got,
				test.want,
			)
		}
	}
}
