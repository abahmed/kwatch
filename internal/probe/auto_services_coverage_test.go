package probe

import (
	"context"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
)

func TestAutoServiceQueueUsesInformerCache(t *testing.T) {
	index := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
			{Name: "http", Port: 8080},
		}},
	}
	if err := index.Add(service); err != nil {
		t.Fatal(err)
	}
	monitor := newTestMonitor(config.ActiveProbeMonitor{
		ExcludeNamespaces: []string{"system"},
	}, nil, &http.Client{})
	monitor.serviceLister = corev1listers.NewServiceLister(index)
	jobs := make(chan serviceProbe, 2)
	current := make(map[string]autoProbeTarget)
	complete := monitor.queueNamespaceProbes(
		context.Background(), "apps", nil, 500, jobs, current,
	)
	close(jobs)
	if !complete || len(current) != 2 {
		t.Fatalf("queue result = %#v, %t", current, complete)
	}
	if len(jobs) != 1 {
		t.Fatalf("queued probes = %d, want 1", len(jobs))
	}
	if got := monitor.excludedNamespaces(); !got["system"] {
		t.Fatal("excluded namespace was not copied")
	}
	monitor.watchAll = false
	monitor.namespaces = nil
	if namespaces, watchAll, _ := monitor.namespaceSnapshot(); watchAll ||
		len(namespaces) != 0 {
		t.Fatalf("namespace snapshot = %v/%t", namespaces, watchAll)
	}
	monitor.checkServices(context.Background())
}

func TestAutoServiceProbeAndRemovedTargetRecovery(t *testing.T) {
	sink := &probeSink{}
	monitor := newTestMonitor(config.ActiveProbeMonitor{}, sink, &http.Client{})
	monitor.checkServiceProbe(context.Background(), serviceProbe{
		port:  corev1.ServicePort{Name: "tcp", Port: 1},
		owner: "service/apps/api/1", address: "127.0.0.1:1",
	})
	monitor.autoTargets = map[string]autoProbeTarget{
		"auto-service/apps/old/80": {owner: "service/apps/old/80"},
	}
	monitor.reconcileAutoTargets(map[string]autoProbeTarget{})
	if len(sink.resolved) != 2 {
		t.Fatalf("removed target resolutions = %d", len(sink.resolved))
	}
	if sink.resolved[0].reason != constant.ReasonActiveProbeFailure {
		t.Fatalf("removed target reason = %q", sink.resolved[0].reason)
	}
}

func TestProbeStartStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	monitor := newTestMonitor(config.ActiveProbeMonitor{Enabled: true}, nil,
		&http.Client{})
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !monitor.started {
		t.Fatal("Start() did not mark monitor started")
	}
}
