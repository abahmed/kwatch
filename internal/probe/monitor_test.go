package probe

import (
	"testing"

	kwcontext "github.com/abahmed/kwatch/internal/context"
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
