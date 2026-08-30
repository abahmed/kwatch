package kubeletmetrics

import "testing"

func TestMetricLineParsesLabelsAndTimestamp(t *testing.T) {
	name, labels, value, ok := metricLine(`container_cpu_cfs_periods_total{pod_namespace="app",pod_name="web",container_name="api"} 42 1700000000`)
	if !ok {
		t.Fatal("metricLine rejected a valid sample")
	}
	if name != "container_cpu_cfs_periods_total" || value != 42 {
		t.Fatalf("unexpected sample: %q %v", name, value)
	}
	for key, want := range map[string]string{"namespace": "app", "pod": "web", "container": "api"} {
		if labels[key] != want {
			t.Errorf("labels[%q] = %q, want %q", key, labels[key], want)
		}
	}
}

func TestParseCountersIgnoresCommentsAndUnrelatedMetrics(t *testing.T) {
	body := []byte(`# HELP unrelated ignored
container_cpu_cfs_periods_total{namespace="app",pod="web",container="api"} 100
container_cpu_cfs_throttled_periods_total{namespace="app",pod="web",container="api"} 25
container_memory_working_set_bytes{namespace="app",pod="web",container="api"} 999`)
	got := parseCounters(body)["app/web/api"]
	if got.Periods != 100 || got.Throttled != 25 {
		t.Fatalf("unexpected counters: %+v", got)
	}
	if len(parseCounters(body)) != 1 {
		t.Fatalf("unrelated metrics should not create counter entries")
	}
}

func TestParseNamedCountersSumsOperationSeries(t *testing.T) {
	body := []byte(`kubelet_runtime_operations_errors_total{operation_type="RunPodSandbox"} 2
kubelet_runtime_operations_errors_total{operation_type="StopContainer"} 3
kubelet_runtime_operations_total{operation_type="RunPodSandbox"} 100`)
	got := sumCounters(parseNamedCounters(body, "kubelet_runtime_operations_errors_total"))
	if got != 5 {
		t.Fatalf("sumCounters = %v, want 5", got)
	}
}

func TestMaxPSIHandlesMissingResources(t *testing.T) {
	if got := maxPSI(nil, &psiStats{Some: psiWindow{Avg10: 12.5}, Full: psiWindow{Avg10: 4}}); got != 12.5 {
		t.Fatalf("maxPSI = %v, want 12.5", got)
	}
}

func TestSnapshotNode(t *testing.T) {
	tests := []struct {
		key  string
		want string
		ok   bool
	}{
		{key: "network/node-a", want: "node-a", ok: true},
		{key: "runtime/node-b", want: "node-b", ok: true},
		{key: "cpu/node-c/app/web/api", want: "node-c", ok: true},
		{key: "unknown/node-d", ok: false},
	}
	for _, test := range tests {
		got, ok := snapshotNode(test.key)
		if got != test.want || ok != test.ok {
			t.Errorf("snapshotNode(%q) = %q, %v; want %q, %v", test.key, got, ok, test.want, test.ok)
		}
	}
}
