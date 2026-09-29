package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/rbac"
)

// reportPermissions publishes each permission sweep as the "rbac" health
// component with a bounded reason code, and logs what is missing.
func reportPermissions(server *health.HealthServer) func(rbac.Status) {
	return func(status rbac.Status) {
		for _, access := range status.Missing {
			klog.InfoS("permission missing", "component", "rbac",
				"group", access.Resource.Group,
				"resource", access.Resource.Name,
				"url", access.NonResourceURL, "verb", access.Verb,
				"namespace", access.Namespace,
				"required", access.Required)
		}
		state, reason := permissionState(status)
		server.SetComponentStatus("rbac", state, reason, reason == "")
	}
}

func permissionState(status rbac.Status) (string, string) {
	switch {
	case status.Unavailable:
		return "degraded", "api_unavailable"
	case status.RequiredMissing():
		return "degraded", "permission_denied"
	case len(status.Missing) > 0:
		return "degraded", "optional_permission_denied"
	default:
		return "running", ""
	}
}
