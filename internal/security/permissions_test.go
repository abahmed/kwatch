package security

import (
	"context"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

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
	if !hasNonResourcePermission(cluster, "/readyz", "get") {
		t.Fatal("control-plane monitoring requires /readyz access")
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

func TestInfrastructureConfigMapPermissionsAreNamed(t *testing.T) {
	permissions := infrastructurePermissions(config.DefaultConfig())
	wanted := persistenceConfigMapNames()

	for _, name := range wanted {
		for _, verb := range []string{"get", "update", "patch"} {
			found := false
			for _, permission := range permissions {
				if permission.Resource == "configmaps" &&
					permission.Name == name && permission.Verb == verb {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing named ConfigMap permission: %s %s", verb, name)
			}
		}
	}

	for _, permission := range permissions {
		if permission.Resource == "configmaps" &&
			permission.Verb != "create" && permission.Name == "" {
			t.Fatalf("non-create ConfigMap permission is not named: %+v", permission)
		}
	}
}

func TestAllowedSendsResourceName(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor(
		"create",
		"selfsubjectaccessreviews",
		func(action ktesting.Action) (bool, runtime.Object, error) {
			create := action.(ktesting.CreateAction)
			review := create.GetObject().(*authorizationv1.SelfSubjectAccessReview)
			if review.Spec.ResourceAttributes.Name != "kwatch-state" {
				t.Fatalf("resource name was not sent: %+v",
					review.Spec.ResourceAttributes)
			}
			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{
					Allowed: true,
				},
			}, nil
		},
	)

	monitor := New(client)
	allowed, err := monitor.allowed(context.Background(), Permission{
		Name:     "kwatch-state",
		Resource: "configmaps",
		Verb:     "update",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("named permission was unexpectedly denied")
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

func hasNonResourcePermission(
	permissions []Permission, path, verb string,
) bool {
	for _, permission := range permissions {
		if permission.NonResourceURL == path && permission.Verb == verb {
			return true
		}
	}
	return false
}
