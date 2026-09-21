package kubeletmetrics

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

type telemetrySink struct {
	processed []*model.Observation
	resolved  []*model.Observation
}

func (s *telemetrySink) Process(
	obs *model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.processed = append(s.processed, obs)
	return nil, model.ActionCreate
}

func (s *telemetrySink) Resolve(model.ObjectRef, string) {}

func (s *telemetrySink) ResolveObserved(obs *model.Observation) {
	s.resolved = append(s.resolved, obs)
}

type telemetryStore struct {
	data []byte
	err  error
}

func (s *telemetryStore) LoadTelemetryState(context.Context) ([]byte, error) {
	return s.data, s.err
}

func (s *telemetryStore) SaveTelemetryState(
	_ context.Context, data []byte,
) error {
	s.data = append([]byte(nil), data...)
	return s.err
}

func TestObserveUsesFailureAndRecoveryThresholds(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{
		FailureThreshold: 2, RecoveryThreshold: 2,
	}, nil)
	monitor.now = func() time.Time { return now }
	reports, resolves := 0, 0
	for range 2 {
		monitor.observe("network/node-a", true, func() { reports++ }, func() {})
	}
	if reports != 1 {
		t.Fatalf("reports = %d, want 1", reports)
	}
	for range 2 {
		monitor.observe("network/node-a", false, func() {}, func() { resolves++ })
	}
	if resolves != 1 {
		t.Fatalf("resolves = %d, want 1", resolves)
	}
}

func TestPruneSignalStateRemovesStaleEntries(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{
		IntervalSeconds: 60,
	}, nil)
	monitor.now = func() time.Time { return now }
	monitor.stateSeen = map[string]time.Time{
		"old": now.Add(-11 * time.Minute), "new": now,
	}
	monitor.failures = map[string]int{"old": 1, "new": 1}
	monitor.successes = map[string]int{"old": 1, "new": 1}
	monitor.failing = map[string]bool{"old": true, "new": true}
	monitor.baselines = map[string]usageBaseline{
		"old": {Updated: now.Add(-11 * time.Minute)},
		"new": {Updated: now},
	}
	monitor.pruneSignalState()
	if _, ok := monitor.stateSeen["old"]; ok {
		t.Fatal("stale signal state was retained")
	}
	if _, ok := monitor.baselines["old"]; ok {
		t.Fatal("stale baseline was retained")
	}
	if _, ok := monitor.stateSeen["new"]; !ok {
		t.Fatal("current signal state was pruned")
	}
}

func TestSnapshotReportsHealthyPartialAndRBACDenied(t *testing.T) {
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	monitor.endpoint = map[string]endpointStatus{
		"node-a": {Summary: true, CAdvisor: true, Runtime: true},
	}
	if got := monitor.Snapshot(); got.State != "healthy" {
		t.Fatalf("healthy snapshot = %+v", got)
	}
	monitor.endpoint["node-b"] = endpointStatus{Summary: true}
	if got := monitor.Snapshot(); got.State != "partial" {
		t.Fatalf("partial snapshot = %+v", got)
	}
	monitor.endpoint = map[string]endpointStatus{
		"node-a": {RBACDenied: true},
	}
	if got := monitor.Snapshot(); got.State != "rbacDenied" {
		t.Fatalf("RBAC snapshot = %+v", got)
	}
	data, err := monitor.StatusJSON()
	if err != nil {
		t.Fatal(err)
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "rbacDenied" {
		t.Fatalf("status JSON = %+v", status)
	}
}

func TestPersistenceRoundTripClonesState(t *testing.T) {
	store := &telemetryStore{}
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{
		PersistState: true,
	}, nil)
	monitor.store = store
	monitor.previous["network/node-a"] = metricSnapshot{At: now}
	monitor.failures["network/node-a"] = 2
	monitor.baselines["pod/app"] = usageBaseline{Updated: now, Samples: 4}
	monitor.saveState(context.Background())
	if len(store.data) == 0 {
		t.Fatal("state was not saved")
	}
	loaded := newTestMonitor(nil, config.KubeletTelemetryMonitor{
		PersistState: true,
	}, nil)
	loaded.store = store
	if err := loaded.loadState(context.Background()); err != nil {
		t.Fatal(err)
	}
	if loaded.failures["network/node-a"] != 2 ||
		loaded.baselines["pod/app"].Samples != 4 {
		t.Fatalf("state did not round-trip: %+v", loaded)
	}
	loaded.previous["network/node-a"] = metricSnapshot{}
	if monitor.previous["network/node-a"].At != now {
		t.Fatal("saved state shared mutable maps with monitor")
	}
}

func TestConfigureSourcesCopiesNamespaceScope(t *testing.T) {
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	namespaces := []string{"apps"}
	if err := monitor.ConfigureSources(Sources{
		Namespaces: namespaces,
	}); err != nil {
		t.Fatal(err)
	}
	namespaces[0] = "changed"
	if monitor.namespaces[0] != "apps" {
		t.Fatal("source configuration retained caller slice")
	}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second source configuration succeeded")
	}
}

func TestPodsUsesListerAndNamespaceFilter(t *testing.T) {
	index := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, pod := range []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "system", Namespace: "kube-system"}},
	} {
		if err := index.Add(pod); err != nil {
			t.Fatal(err)
		}
	}
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	monitor.watchAll = false
	monitor.namespaces = []string{"apps"}
	monitor.allowed = func(namespace string) bool { return namespace == "apps" }
	monitor.podLister = corev1listers.NewPodLister(index)
	pods := monitor.pods(context.Background())
	if len(pods) != 1 || pods["apps/api"] == nil {
		t.Fatalf("unexpected pod cache: %+v", pods)
	}
}

func TestObservationStateKeyIncludesSubjectAndReason(t *testing.T) {
	obs := &model.Observation{
		Subject: model.ObjectRef{Kind: "node", Name: "node-a"},
		Reason:  constant.ReasonNodeNetworkErrors,
	}
	if got := observationStateKey(obs); got !=
		"node//node-a:"+constant.ReasonNodeNetworkErrors {
		t.Fatalf("observation key = %q", got)
	}
	if observationStateKey(nil) != "" {
		t.Fatal("nil observation should have an empty state key")
	}
}

func TestCheckNetworkReportsAfterCounterIncrease(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	sink := &telemetrySink{}
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{
		NetworkErrorRateWarning: 1, NetworkErrorRateCritical: 5,
	}, sink)
	monitor.now = func() time.Time { return now }
	monitor.checkNetwork("node-a", networkStats{RxErrors: 1})
	now = now.Add(2 * time.Second)
	monitor.checkNetwork("node-a", networkStats{RxErrors: 21})
	if len(sink.processed) != 1 || sink.processed[0].Reason !=
		constant.ReasonNodeNetworkErrors {
		t.Fatalf("network observations = %+v", sink.processed)
	}
}

func TestAdaptiveUsageThresholdLearnsAfterFiveSamples(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	monitor.now = func() time.Time { return now }
	for range 5 {
		monitor.adaptiveUsageThreshold("pod/app", 60, 50, 90)
	}
	warning, critical := monitor.adaptiveUsageThreshold("pod/app", 60, 50, 90)
	if warning <= 50 || critical != 90 {
		t.Fatalf("learned thresholds = %v, %v", warning, critical)
	}
	warning, critical = monitor.adaptiveUsageThreshold("pod/app", 95, 50, 90)
	if warning != 50 || critical != 90 {
		t.Fatalf("critical sample changed thresholds = %v, %v", warning, critical)
	}
}

func TestReportUsageEmitsContainerSignalAndRecovery(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	sink := &telemetrySink{}
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, sink)
	monitor.now = func() time.Time { return now }
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api", UID: "uid-1",
	}, Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "app",
		Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("100Mi"),
		}},
	}}}}
	monitor.reportUsage(
		pod, "app", 90, 50, 80,
		constant.ReasonContainerMemoryHigh, "90Mi", "100Mi", "memory",
	)
	if len(sink.processed) != 1 ||
		sink.processed[0].Severity != model.SeverityCritical {
		t.Fatalf("container signal = %+v", sink.processed)
	}
	monitor.reportUsage(
		pod, "app", 20, 50, 80,
		constant.ReasonContainerMemoryHigh, "20Mi", "100Mi", "memory",
	)
	if len(sink.resolved) != 1 {
		t.Fatalf("container recovery = %+v", sink.resolved)
	}
}

func TestMetricIdentitySplitsContainerKey(t *testing.T) {
	namespace, pod, container := metricIdentity("apps/api/app")
	if namespace != "apps" || pod != "api" || container != "app" {
		t.Fatalf("metric identity = %q, %q, %q", namespace, pod, container)
	}
	namespace, pod, container = metricIdentity("invalid")
	if namespace != "" || pod != "" || container != "" {
		t.Fatalf("invalid metric identity = %q, %q, %q", namespace, pod, container)
	}
}
