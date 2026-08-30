package kubeletmetrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/config"
)

func testConfig() config.KubeletTelemetryMonitor {
	return config.KubeletTelemetryMonitor{
		Enabled: true, IntervalSeconds: 60, FailureThreshold: 1, RecoveryThreshold: 1,
		MemoryWarningPercent: 90, MemoryCriticalPercent: 100,
		EphemeralStorageWarningPercent: 90, EphemeralStorageCriticalPercent: 95,
		CPUWarningPercent: 90, CPUCriticalPercent: 100,
		CPUThrottlingWarningPercent: 25, CPUThrottlingCriticalPercent: 50,
		PSIWarningPercent: 20, PSICriticalPercent: 50,
		NetworkErrorRateWarning: 1, NetworkErrorRateCritical: 10,
		RuntimeErrorRateWarning: 1, RuntimeErrorRateCritical: 10,
	}
}

func testNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestKubeletProxyEndpointsAndRetry(t *testing.T) {
	var summaryCalls, cadvisorCalls, runtimeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/nodes/node-a/proxy/stats/summary":
			summaryCalls.Add(1)
			_, _ = w.Write([]byte(`{"node":{}}`))
		case "/api/v1/nodes/node-a/proxy/metrics/cadvisor":
			call := cadvisorCalls.Add(1)
			if call == 1 {
				http.Error(w, "temporary kubelet failure", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("# HELP test ignored\n"))
		case "/api/v1/nodes/node-a/proxy/metrics":
			runtimeCalls.Add(1)
			_, _ = w.Write([]byte(`kubelet_runtime_operations_errors_total{operation_type="RunPodSandbox"} 0`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := kubernetes.NewForConfig(&rest.Config{
		Host: server.URL,
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{Version: "v1"},
			NegotiatedSerializer: scheme.Codecs,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	monitor := New(client, testConfig(), nil)
	monitor.checkSummary(context.Background(), testNode("node-a"), nil)
	monitor.checkCadvisor(context.Background(), testNode("node-a"))
	monitor.checkRuntimeMetrics(context.Background(), testNode("node-a"))

	if summaryCalls.Load() != 1 || cadvisorCalls.Load() != 2 || runtimeCalls.Load() != 1 {
		t.Fatalf("unexpected kubelet calls: summary=%d cadvisor=%d runtime=%d", summaryCalls.Load(), cadvisorCalls.Load(), runtimeCalls.Load())
	}
}
