package kubeletmetrics

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// cfsSeries renders one cAdvisor CFS counter line for container app/web/api.
func cfsSeries(metric, id string, value int) string {
	return fmt.Sprintf(
		`%s{namespace="app",pod="web",container="api",id=%q} %d`,
		metric, id, value,
	)
}

// A container that just restarted reports two cgroups. Mixing the throttled
// count of one with the period count of the other produced ratios above
// 100%; both numbers must come from the same series.
func TestParseCountersPairsSeriesByCgroup(t *testing.T) {
	body := []byte(strings.Join([]string{
		cfsSeries("container_cpu_cfs_periods_total", "/old", 1000),
		cfsSeries("container_cpu_cfs_throttled_periods_total", "/old", 900),
		cfsSeries("container_cpu_cfs_periods_total", "/new", 10),
		cfsSeries("container_cpu_cfs_throttled_periods_total", "/new", 2),
	}, "\n"))
	got := parseCounters(body)["app/web/api"]
	if got.Periods != 1000 || got.Throttled != 900 {
		t.Fatalf("expected the longest-running cgroup's pair, got %+v", got)
	}
	if got.Throttled > got.Periods {
		t.Fatalf("throttled periods exceed total periods: %+v", got)
	}
}

func TestParseCountersWithoutIDLabelStillPairs(t *testing.T) {
	body := []byte(strings.Join([]string{
		`container_cpu_cfs_periods_total{namespace="app",pod="web",` +
			`container="api"} 100`,
		`container_cpu_cfs_throttled_periods_total{namespace="app",` +
			`pod="web",container="api"} 25`,
	}, "\n"))
	got := parseCounters(body)["app/web/api"]
	if got.Periods != 100 || got.Throttled != 25 {
		t.Fatalf("unexpected counters: %+v", got)
	}
}

// Kubelet-derived incidents must land on the same workload key as the rest of
// the pipeline, or one Deployment's replicas arrive as separate alerts.
func TestPodOwnerPrefersResolver(t *testing.T) {
	m := New(nil, testConfig(), nil)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "web-7d8d7-abc", Namespace: "app",
		OwnerReferences: []metav1.OwnerReference{
			{Kind: "ReplicaSet", Name: "web-7d8d7"},
		},
	}}

	got := m.podOwner(pod, "app", pod.Name)
	if got.Name != "app/web-7d8d7-abc" {
		t.Fatalf("without a resolver expected pod-keyed owner, got %q", got)
	}

	m.SetOwnerResolver(observe.OwnerFunc(
		func(*corev1.Pod) model.ObjectRef {
			return model.ObjectRef{Kind: "Deployment", Name: "web"}
		},
	))
	if got := m.podOwner(pod, "app", pod.Name); got.Name != "web" {
		t.Fatalf("expected resolver's owner, got %q", got)
	}

	m.SetOwnerResolver(observe.OwnerFunc(
		func(*corev1.Pod) model.ObjectRef { return model.ObjectRef{} },
	))
	got = m.podOwner(pod, "app", pod.Name)
	if got.Name != "app/web-7d8d7-abc" {
		t.Fatalf("an unresolved owner must fall back, got %q", got)
	}
}

func TestPodOwnerOwnerlessPodIsItself(t *testing.T) {
	m := New(nil, testConfig(), nil)
	m.SetOwnerResolver(observe.OwnerFunc(
		func(*corev1.Pod) model.ObjectRef { return model.ObjectRef{} },
	))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "solo", Namespace: "app",
	}}
	if got := m.podOwner(pod, "app", "solo"); got.Name != "solo" {
		t.Fatalf("expected the pod's own name, got %q", got)
	}
	if got := m.podOwner(nil, "app", "gone"); got.Name != "app/gone" {
		t.Fatalf("expected namespace/pod for an unknown pod, got %q", got)
	}
}
