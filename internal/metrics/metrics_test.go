package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scrape(t *testing.T, r *Registry) string {
	t.Helper()
	rr := httptest.NewRecorder()
	r.Handler().ServeHTTP(
		rr, httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	return rr.Body.String()
}

func TestHandlerUsesStableOrderAndGETOnly(t *testing.T) {
	r := &Registry{}
	r.IncIncident("announce")
	r.IncIncident("update")
	body := scrape(t, r)
	if strings.Index(body, `action="announce"`) >
		strings.Index(body, `action="update"`) {
		t.Fatal("incident metrics are not in stable order")
	}
	post := httptest.NewRecorder()
	r.Handler().ServeHTTP(
		post, httptest.NewRequest(http.MethodPost, "/metrics", nil),
	)
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", post.Code)
	}
}

func TestHandlerExposesStableMetricContract(t *testing.T) {
	r := &Registry{}
	r.IncIncident("resolve")
	r.IncIncident("resolve")
	r.IncIncident("unbounded-value")
	r.IncidentsOpen.Store(3)
	r.NotificationsTotal.Store(4)
	r.NotificationsDropped.Store(5)
	r.DeliveryRetries.Store(6)
	r.ShutdownTimeouts.Store(8)
	r.SourceUnavailable.Store(9)
	r.LeadershipAcquisitions.Store(10)
	r.LeadershipLosses.Store(11)
	r.LeaderTakeovers.Store(12)
	r.InformerHandlerPanics.Store(13)
	r.RenderedDetailsOmitted.Store(14)
	r.RedactedValues.Store(15)
	r.IncTelemetryFailure("state_read")
	r.IncTelemetryFailure("arbitrary error text")

	body := scrape(t, r)
	for _, metric := range []string{
		"# TYPE kwatch_incidents_open gauge",
		"kwatch_incidents_open 3",
		`kwatch_incidents_total{action="announce"} 0`,
		`kwatch_incidents_total{action="resolve"} 2`,
		"# TYPE kwatch_delivery_notifications_total counter",
		"kwatch_delivery_notifications_total 4",
		"kwatch_delivery_dropped_total 5",
		"kwatch_delivery_retries_total 6",
		"kwatch_shutdown_timeouts_total 8",
		"kwatch_source_unavailable_total 9",
		"kwatch_leadership_acquisitions_total 10",
		"kwatch_leadership_losses_total 11",
		"kwatch_leader_takeovers_total 12",
		"kwatch_informer_handler_panics_total 13",
		"kwatch_rendered_details_omitted_total 14",
		"kwatch_redacted_values_total 15",
		`kwatch_telemetry_failures_total{reason="state_read"} 1`,
		`kwatch_telemetry_failures_total{reason="network"} 1`,
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics output is missing %q", metric)
		}
	}
}
