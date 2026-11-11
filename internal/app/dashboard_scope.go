package app

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/scope"
)

// dashboardScope is the namespace scope the dashboard shows and scopes
// its releases by: the allowed and forbidden lists, and the namespace
// label selector through the same rule findings are scoped by. Reasons,
// silences and maintenance are about findings, not namespaces, and are
// left out.
func dashboardScope(
	runtime config.ScopeRuntime, now clock.Clock,
) dashboard.Scope {
	out := dashboard.Scope{
		Watched:  runtime.AllowedNamespaces(),
		Excluded: runtime.ForbiddenNamespaces(),
	}
	namespaces, err := scope.New(runtime, nil, false, now.Now)
	if err != nil {
		// The pipeline refuses the same configuration with this error.
		klog.ErrorS(err, "dashboard: namespace scope not built; "+
			"releases are scoped by the namespace lists only",
			"component", "dashboard-server")
		return out
	}
	out.Allowed = namespaces.NamespaceAllowed
	return out
}
