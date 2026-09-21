package metricsapi

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

type metricsSink struct {
	processed []*model.Observation
	resolved  []*model.Observation
}

func (s *metricsSink) Process(
	obs *model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.processed = append(s.processed, obs)
	return nil, model.ActionCreate
}

func (s *metricsSink) Resolve(model.ObjectRef, string) {}

func (s *metricsSink) ResolveObserved(obs *model.Observation) {
	s.resolved = append(s.resolved, obs)
}

func TestUsageSignalClassifiesThresholds(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "apps",
	}}
	owner := observe.SelfOwner("Pod", "apps", "api")
	limit := resource.MustParse("100Mi")

	tests := []struct {
		name    string
		usage   string
		want    model.Severity
		wantNil bool
	}{
		{name: "below warning", usage: "20Mi", wantNil: true},
		{name: "warning", usage: "60Mi", want: model.SeverityWarning},
		{name: "critical", usage: "90Mi", want: model.SeverityCritical},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usage := resource.MustParse(test.usage)
			obs := usageSignal(
				pod, "app", &usage, &limit, 50, 80,
				constant.ReasonContainerMemoryHigh, "memory", owner,
			)
			if (obs == nil) != test.wantNil {
				t.Fatalf("usageSignal() nil = %v", obs == nil)
			}
			if obs != nil && obs.Severity != test.want {
				t.Fatalf("severity = %q, want %q", obs.Severity, test.want)
			}
		})
	}
}

func TestUsageSignalRejectsMissingLimitOrUsage(t *testing.T) {
	usage := resource.MustParse("10Mi")
	if got := usageSignal(
		nil, "app", &usage, nil, 50, 80, "reason", "memory", model.ObjectRef{},
	); got != nil {
		t.Fatal("missing limit should not create a signal")
	}
	zero := resource.MustParse("0")
	if got := usageSignal(
		nil, "app", &usage, &zero, 50, 80, "reason", "memory", model.ObjectRef{},
	); got != nil {
		t.Fatal("zero limit should not create a signal")
	}
	limit := resource.MustParse("10Mi")
	if got := usageSignal(
		nil, "app", nil, &limit, 50, 80, "reason", "memory", model.ObjectRef{},
	); got != nil {
		t.Fatal("missing usage should not create a signal")
	}
}

func TestContainerLimitsIncludesInitAndApplicationContainers(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		InitContainers: []corev1.Container{{
			Name: "init", Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Mi"),
				},
			},
		}},
		Containers: []corev1.Container{{Name: "app"}},
	}}
	limits := containerLimits(pod)
	if _, ok := limits["init"]; !ok {
		t.Fatal("init container limit was omitted")
	}
	if _, ok := limits["app"]; !ok {
		t.Fatal("application container limit was omitted")
	}
}

func TestParseQuantityRejectsEmptyAndMalformedValues(t *testing.T) {
	if _, ok := parseQuantity(""); ok {
		t.Fatal("empty quantity should be rejected")
	}
	if _, ok := parseQuantity("not-a-quantity"); ok {
		t.Fatal("malformed quantity should be rejected")
	}
	if got, ok := parseQuantity("10Mi"); !ok || got.String() != "10Mi" {
		t.Fatalf("parseQuantity() = %v, %v", got, ok)
	}
}

func TestProcessMetricReportsAndResolvesEachResource(t *testing.T) {
	sink := &metricsSink{}
	monitor := &Monitor{
		incidentSink: sink,
		cfg: config.RuntimeMetricsMonitor{
			MemoryWarningPercent: 50, MemoryCriticalPercent: 80,
			CPUWarningPercent: 50, CPUCriticalPercent: 80,
		},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}}
	seen := make(map[string]bool)
	monitor.processMetric(
		pod, "app", map[string]string{
			"memory": "90Mi", "cpu": "10m",
		}, corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("100Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		}, seen,
	)
	if len(sink.processed) != 1 || sink.processed[0].Reason !=
		constant.ReasonContainerMemoryHigh {
		t.Fatalf("unexpected processed observations: %+v", sink.processed)
	}
	if len(sink.resolved) != 1 || sink.resolved[0].Reason !=
		constant.ReasonContainerCPUHigh {
		t.Fatalf("unexpected resolved observations: %+v", sink.resolved)
	}
	if !seen["app"] {
		t.Fatal("processed container was not marked seen")
	}
}
