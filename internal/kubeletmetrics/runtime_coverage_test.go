package kubeletmetrics

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
)

func TestStartSkipsDisabledMonitorAndStopsCanceledMonitor(t *testing.T) {
	disabled := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	if err := disabled.Start(context.Background()); err != nil {
		t.Fatalf("disabled Start() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{
		Enabled: true,
	}, nil)
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("nil-client Start() error = %v", err)
	}
	monitor = newTestMonitor(fake.NewSimpleClientset(),
		config.KubeletTelemetryMonitor{Enabled: true}, nil)
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("canceled client Start() error = %v", err)
	}
}

func TestEndpointAndSnapshotPruning(t *testing.T) {
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	monitor.endpoint = map[string]endpointStatus{"old": {Summary: true}}
	monitor.recordEndpoint("node-a", "summary", nil)
	monitor.recordEndpoint("node-a", "cadvisor", nil)
	monitor.recordEndpoint("node-a", "runtime", nil)
	monitor.recordEndpoint("node-a", "summary", apierrors.NewForbidden(
		schema.GroupResource{Resource: "nodes"}, "node-a",
		errors.New("denied")))
	if !monitor.endpoint["node-a"].RBACDenied {
		t.Fatal("forbidden endpoint was not recorded")
	}
	monitor.previous = map[string]metricSnapshot{
		"network/node-a": {}, "network/old": {}, "bad": {},
	}
	monitor.pruneSnapshots([]corev1.Node{{ObjectMeta: metav1.ObjectMeta{
		Name: "node-a",
	}}})
	if _, ok := monitor.previous["network/old"]; ok {
		t.Fatal("stale snapshot was retained")
	}
	if _, ok := monitor.endpoint["old"]; ok {
		t.Fatal("stale endpoint was retained")
	}
	monitor.resetEndpointStatus([]corev1.Node{{ObjectMeta: metav1.ObjectMeta{
		Name: "node-a",
	}}})
	if got := monitor.TelemetryStatus(); got.Nodes != 1 {
		t.Fatalf("TelemetryStatus() = %+v", got)
	}
}

func TestNodesUsesInformerCacheAndEmptyScopedPods(t *testing.T) {
	nodeIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := nodeIndex.Add(&corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "node-a",
	}}); err != nil {
		t.Fatal(err)
	}
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	monitor.nodeLister = corev1listers.NewNodeLister(nodeIndex)
	nodes, err := monitor.nodes(context.Background())
	if err != nil || len(nodes) != 1 || nodes[0].Name != "node-a" {
		t.Fatalf("nodes = %#v, %v", nodes, err)
	}
	monitor.watchAll = false
	monitor.namespaces = nil
	if pods := monitor.pods(context.Background()); len(pods) != 0 {
		t.Fatalf("scoped pods = %#v", pods)
	}
	monitor.podLister = corev1listers.NewPodLister(
		cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{}),
	)
	monitor.watchAll = true
	monitor.podCacheAt = time.Time{}
	if pods := monitor.pods(context.Background()); pods == nil {
		t.Fatal("pods returned nil for empty cache")
	}
}
