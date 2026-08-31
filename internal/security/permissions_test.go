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

func hasPermission(permissions []Permission, resource, group string) bool {
	for _, permission := range permissions {
		if permission.Resource == resource && permission.Group == group {
			return true
		}
	}
	return false
}
