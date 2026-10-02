package health

import (
	"errors"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestSetComponentStatusKeepsPermissionReasons(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})

	for _, reason := range []string{
		"api_unavailable", "permission_denied",
		"optional_permission_denied", "storage_reset", "storage_over_cap",
		"heartbeat_failed", "kubelet_unreachable",
		"kubelet_partially_unreachable", "config_overlay_invalid",
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

func TestSetComponentErrorReasonsStayInVocabulary(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"stalled component", errors.New("component stalled"),
			"component_stalled"},
		{"stopped component", errors.New("workers stopped unexpectedly"),
			"component_stopped"},
		{"raw failure", errors.New("open /var/lib/kwatch: denied"),
			"component_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewHealthServerWithClock(
				config.HealthCheck{}, clock.RealClock{})

			server.SetComponentError("pipeline", tc.err)

			got := server.ComponentStatuses()["pipeline"].Reason
			if got != tc.want || normalizeReason(got) != got {
				t.Fatalf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClearComponentStatusForgetsComponent(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})
	server.SetComponentError("heartbeat", errors.New("boom"))

	server.ClearComponentStatus("heartbeat")

	if _, ok := server.ComponentStatuses()["heartbeat"]; ok {
		t.Fatal("status still published")
	}
	if _, ok := server.ComponentErrors()["heartbeat"]; ok {
		t.Fatal("error still published")
	}
}
