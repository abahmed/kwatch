package kubeletmetrics

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestIncidentHelpersUseCachedPodsAndOwners(t *testing.T) {
	sink := &telemetrySink{}
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, sink)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}}
	monitor.podCache["apps/api"] = pod
	monitor.report("node-a", constant.ReasonNodeNetworkErrors,
		model.SeverityWarning, "network")
	monitor.reportContainer(pod, "app", model.ObjectRef{Kind: "Deployment",
		Name: "api"}, model.SeverityCritical, 25)
	monitor.resolve("node-a", constant.ReasonNodeNetworkErrors)
	monitor.resolveContainer("apps", "api", "app")
	if len(sink.processed) != 2 || len(sink.resolved) != 1 {
		t.Fatalf("processed/resolved = %d/%d", len(sink.processed),
			len(sink.resolved))
	}
	if got := monitor.cachedPod("apps", "api"); got == pod {
		t.Fatal("cachedPod returned mutable cache pointer")
	}
	if got := monitor.cachedPod("apps", "missing"); got != nil {
		t.Fatal("missing cached pod returned a value")
	}
}

func TestPodOwnerFallsBackForMissingAndUnresolvedPods(t *testing.T) {
	monitor := newTestMonitor(nil, config.KubeletTelemetryMonitor{}, nil)
	if got := monitor.podOwner(nil, "apps", "api"); got.Name != "apps/api" {
		t.Fatalf("missing pod owner = %+v", got)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api", OwnerReferences: []metav1.OwnerReference{{
			Kind: "ReplicaSet", Name: "api-1",
		}},
	}}
	if got := monitor.podOwner(pod, "apps", "api"); got.Name != "apps/api" {
		t.Fatalf("unresolved owner = %+v", got)
	}
}
