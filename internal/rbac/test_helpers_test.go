package rbac

import (
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func newTestConfiguredMonitor(
	client kubernetes.Interface,
	cfg *config.Config,
) *Monitor {
	return NewWithRuntimeConfig(
		client, config.RuntimeConfigFor(cfg), clock.RealClock{},
	)
}

func newTestFullMonitor(client kubernetes.Interface) *Monitor {
	return &Monitor{
		client:                client,
		clusterPermissions:    clusterPermissions(),
		namespacedPermissions: namespacedPermissions(),
		now:                   clock.RealClock{}.Now,
	}
}

func testPermissionsForConfig(
	cfg *config.Config,
) ([]Permission, []Permission) {
	return permissionsForRuntime(config.RuntimeConfigFor(cfg))
}

func testInfrastructurePermissions(cfg *config.Config) []Permission {
	return infrastructurePermissionsForRuntime(config.RuntimeConfigFor(cfg))
}
