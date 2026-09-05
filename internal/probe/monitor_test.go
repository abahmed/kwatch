package probe

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func TestServiceDNS(t *testing.T) {
	service, namespace, ok := serviceDNS("api.apps.svc.cluster.local.")
	if !ok || service != "api" || namespace != "apps" {
		t.Fatalf("unexpected service DNS result: %q/%q/%v", service, namespace, ok)
	}
	if _, _, ok := serviceDNS("example.com"); ok {
		t.Fatal("external host should not be treated as a Kubernetes service")
	}
}

func TestLinkTargetUsesServiceDependency(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	monitor.linkTarget("http/api", "https://api.apps.svc.cluster.local/health")

	deps := graph.DependenciesOf("activeprobe", "", "http/api")
	if len(deps) != 1 || deps[0] != "service/apps/api" {
		t.Fatalf("unexpected probe dependencies: %v", deps)
	}
}

func TestReplaceAutoTargetsPrunesCounters(t *testing.T) {
	monitor := &Monitor{
		failures:  map[string]int{"old": 2},
		successes: map[string]int{"old": 1},
		autoTargets: map[string]autoProbeTarget{
			"old": {owner: "service/apps/old/80"},
		},
	}

	removed := monitor.replaceAutoTargets(map[string]autoProbeTarget{})

	if len(removed) != 1 || len(monitor.failures) != 0 ||
		len(monitor.successes) != 0 {
		t.Fatalf("stale target state was not pruned: %+v", monitor)
	}
}

func TestServiceProbesBuildStableTarget(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
			Name: "http", Port: 8080,
		}}},
	}
	probes := serviceProbes(service)
	if len(probes) != 1 ||
		probes[0].owner != "service/apps/api/8080" ||
		probes[0].address != "api.apps.svc:8080" {
		t.Fatalf("unexpected probes: %+v", probes)
	}
}

func TestServiceProbesSkipNonTCPPorts(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: "apps"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
			{Name: "dns", Port: 53, Protocol: corev1.ProtocolUDP},
			{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP},
		}},
	}

	probes := serviceProbes(service)
	if len(probes) != 1 || probes[0].port.Port != 8080 {
		t.Fatalf("unexpected probes: %+v", probes)
	}
}

func TestGraphNodeStillUsedBySiblingProbe(t *testing.T) {
	current := map[string]autoProbeTarget{
		"auto-service/apps/api/80": {
			owner:     "service/apps/api/80",
			graphNode: "service/apps/api/80",
		},
	}
	if !graphNodeStillUsed(current, "service/apps/api/80") {
		t.Fatal("TCP probe should keep the shared graph node")
	}
}
