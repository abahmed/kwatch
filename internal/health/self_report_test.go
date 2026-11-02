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

	h.SetDashboardLinks(LinkStatus{Reason: LinksPortMismatch})
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

// Every reason a dashboard link can be off has words, and /status says
// "off" without a reason it does not know.
func TestLinkReasonTextWordsEveryReason(t *testing.T) {
	for _, reason := range []string{LinksNoRoute, LinksPortMismatch,
		LinksUnusableAddress, LinksRoutesForbidden} {
		if LinkReasonText(reason) == "" {
			t.Errorf("no words for %s", reason)
		}
	}
	if got := LinkReasonText("other"); got != "" {
		t.Errorf("LinkReasonText(other) = %q", got)
	}

	h := newStatusServer()
	h.SetDashboardLinks(LinkStatus{Reason: "other"})
	gap, ok := h.linkGap()
	if !ok || gap.Reason != "off" {
		t.Fatalf("gap = %+v, %v", gap, ok)
	}
}
