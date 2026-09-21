package metricsapi

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestStartStopsImmediatelyWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	monitor := &Monitor{}
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !monitor.started {
		t.Fatal("Start() did not mark monitor started")
	}
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
}

func TestNewWithClientAndSweepUseConfiguredScope(t *testing.T) {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{podMetricsGVR: "PodMetricsList"},
	)
	monitor := NewWithClient(dynamicClient, fake.NewSimpleClientset(),
		config.RuntimeMetricsMonitor{}, nil)
	if monitor.metrics == nil || !monitor.watchAll {
		t.Fatal("NewWithClient did not initialize monitor")
	}
	monitor.watchAll = false
	monitor.namespaces = []string{"apps"}
	monitor.allowed = func(namespace string) bool {
		return namespace == "apps"
	}
	monitor.sweep(context.Background())
	if _, err := monitor.listPods(context.Background(), "apps"); err != nil {
		t.Fatalf("listPods() error = %v", err)
	}
}
