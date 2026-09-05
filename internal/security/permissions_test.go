package security

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPermissionsFollowEnabledMonitors(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RolloutMonitor.Enabled = false
	cfg.TlsMonitor.Enabled = false
	cfg.PvcMonitor.Enabled = false
	cluster, namespaced := permissionsForConfig(cfg)

	if hasPermission(namespaced, "deployments", "apps") {
		t.Fatal("disabled rollout monitor should not require deployments")
	}
	if !hasPermission(namespaced, "secrets", "") {
		t.Fatal("graph wiring requires secrets even when TLS monitor is disabled")
	}
	if !hasPermission(cluster, "persistentvolumes", "") {
		t.Fatal("graph wiring requires persistent volumes even when PVC monitor is disabled")
	}
	if !hasPermission(namespaced, "pods/log", "") {
		t.Fatal("enabled log enrichment requires pods/log get access")
	}

	cfg.RolloutMonitor.Enabled = true
	cfg.TlsMonitor.Enabled = true
	cfg.PvcMonitor.Enabled = true
	cluster, namespaced = permissionsForConfig(cfg)
	if !hasPermission(namespaced, "deployments", "apps") ||
		!hasPermission(namespaced, "secrets", "") ||
		!hasPermission(cluster, "persistentvolumes", "") {
		t.Fatal("enabled monitors should require their resources")
	}
}

func TestInfrastructurePermissionsUseRuntimeNamespace(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CrdConfig.Enabled = true
	monitor := NewWithConfig(nil, cfg)
	monitor.SetInfrastructureNamespace("kwatch")

	for _, permission := range monitor.infrastructure {
		if permission.Namespace != "kwatch" {
			t.Fatalf("permission is not runtime-scoped: %+v", permission)
		}
	}
	if !hasPermission(
		monitor.infrastructure, "kwatchconfigs", "kwatch.abahmed.dev",
	) {
		t.Fatal("KwatchConfig permission is missing")
	}
}

func TestOptionalMonitorsRequireTheirRuntimePermissions(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ActiveProbeMonitor.Enabled = true
	cfg.ActiveProbeMonitor.AutoServices = true
	cfg.RuntimeMetricsMonitor.Enabled = true

	_, namespaced := permissionsForConfig(cfg)
	if !hasPermission(namespaced, "services", "") {
		t.Fatal("automatic probes require Service list access")
	}
	if !hasPermission(namespaced, "pods", "metrics.k8s.io") {
		t.Fatal("runtime metrics require PodMetrics list access")
	}
}

func TestDynamicResourcesUseTheirKubernetesScope(t *testing.T) {
	cfg := config.DefaultConfig()
	cluster, namespaced := permissionsForConfig(cfg)

	for _, resource := range []string{
		"volumesnapshots", "gateways", "httproutes", "referencegrants",
	} {
		if !hasPermission(namespaced, resource, resourceGroup(resource)) {
			t.Fatalf("%s should be checked per namespace", resource)
		}
		if hasPermission(cluster, resource, resourceGroup(resource)) {
			t.Fatalf("%s should not be checked as cluster-scoped", resource)
		}
	}
}

func resourceGroup(resource string) string {
	if resource == "volumesnapshots" {
		return "snapshot.storage.k8s.io"
	}
	return "gateway.networking.k8s.io"
}

func hasPermission(permissions []Permission, resource, group string) bool {
	for _, permission := range permissions {
		if permission.Resource == resource && permission.Group == group {
			return true
		}
	}
	return false
}
