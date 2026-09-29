package health

import (
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestSetComponentStatusKeepsPermissionReasons(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})

	for _, reason := range []string{
		"api_unavailable", "permission_denied",
		"optional_permission_denied",
	} {
		server.SetComponentStatus("rbac", "degraded", reason, false)

		if got := server.ComponentStatuses()["rbac"].Reason; got != reason {
			t.Errorf("reason = %q, want %q", got, reason)
		}
	}
}

func TestSetComponentStatusBoundsUnknownReasons(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})

	server.SetComponentStatus("x", "degraded", "raw: secret", false)

	if got := server.ComponentStatuses()["x"].Reason; got !=
		"component_failed" {
		t.Errorf("reason = %q", got)
	}
}
