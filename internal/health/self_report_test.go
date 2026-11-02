package health

import (
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/status"
)

// /status says when alerts carry no dashboard link, and why.
func TestStatusNamesDashboardLinksThatAreOff(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	if _, ok := h.DashboardLinks(); ok {
		t.Fatal("no link state before one is published")
	}

	h.SetDashboardLinks(LinkStatus{Reason: "port_mismatch"})
	body := getStatus(h, http.MethodGet, "/status").Body.String()
	if !strings.Contains(body, "dashboard links in alerts") ||
		!strings.Contains(body, `not its "dashboard" port`) {
		t.Fatalf("body = %q", body)
	}

	h.SetDashboardLinks(LinkStatus{On: true, Address: "https://k"})
	body = getStatus(h, http.MethodGet, "/status").Body.String()
	if strings.Contains(body, "dashboard links") {
		t.Fatalf("links on, still listed: %q", body)
	}
}

// Refused pod logs are a coverage gap on /status, never an incident.
func TestStatusNamesRefusedPodLogs(t *testing.T) {
	h := newStatusServer()
	h.SetStatusSource(func() status.Report { return status.Report{} })
	h.SetComponentStatus(PodLogsComponent, "degraded",
		"optional_permission_denied", false)

	body := getStatus(h, http.MethodGet, "/status").Body.String()

	if !strings.Contains(body, "crash output from pod logs") {
		t.Fatalf("body = %q", body)
	}
}

func TestSetComponentStatusKeepsTrailRestoreFailed(t *testing.T) {
	h := newStatusServer()
	h.SetComponentStatus("delivery-outbox", "degraded",
		"trail_restore_failed", true)

	if got := h.ComponentStatuses()["delivery-outbox"].Reason; got !=
		"trail_restore_failed" {
		t.Fatalf("reason = %q", got)
	}
}
