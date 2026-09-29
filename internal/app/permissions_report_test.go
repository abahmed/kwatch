package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/rbac"
)

func TestReportPermissionsPublishesBoundedReason(t *testing.T) {
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})
	report := reportPermissions(server)

	report(rbac.Status{Missing: []kube.Access{{
		Verb: "list", Namespace: "shop", Required: true,
	}}})
	denied := server.ComponentStatuses()["rbac"]
	report(rbac.Status{})
	healthy := server.ComponentStatuses()["rbac"]

	require.Equal(t, "degraded", denied.State)
	require.Equal(t, "permission_denied", denied.Reason)
	require.Equal(t, "running", healthy.State)
	require.Empty(t, healthy.Reason, "recovery clears the reason")
}
