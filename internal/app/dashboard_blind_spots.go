package app

import (
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/health"
)

// wireDashboardBlindSpots lets the dashboard's kwatch health page show
// the optional parts /health reports as not working (kubelet stats,
// permissions, license, ...), so it never says "working normally"
// while kwatch is half-blind.
func wireDashboardBlindSpots(
	dash *dashboard.Server, server *health.HealthServer,
) {
	if dash == nil || server == nil {
		return
	}
	dash.SetComponentErrors(server.ComponentErrors)
}
