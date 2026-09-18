package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerUsesStableOrderAndGETOnly(t *testing.T) {
	r := &Registry{}
	r.IncidentsCreate.Store(1)
	r.IncidentsUpdate.Store(2)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	r.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Index(body, `action="create"`) > strings.Index(body, `action="update"`) {
		t.Fatal("incident metrics are not in stable order")
	}
	post := httptest.NewRecorder()
	r.Handler().ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", post.Code)
	}
}

func TestHandlerExposesStableMetricContract(t *testing.T) {
	r := &Registry{}
	r.QueueDepth.Store(3)
	r.NotificationsTotal.Store(4)
	r.GraphNodes.Store(5)
	r.DeliveryRetries.Store(6)
	r.PersistenceMigrationErr.Store(7)
	r.ShutdownTimeouts.Store(8)
	r.SourceUnavailable.Store(9)
	r.LeadershipAcquisitions.Store(10)
	r.LeadershipLosses.Store(11)
	r.LeaderTakeovers.Store(12)

	rr := httptest.NewRecorder()
	r.Handler().ServeHTTP(
		rr,
		httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	body := rr.Body.String()
	for _, metric := range []string{
		"# TYPE kwatch_queue_depth gauge",
		"kwatch_queue_depth 3",
		"# TYPE kwatch_notifications_total counter",
		"kwatch_notifications_total 4",
		"kwatch_graph_nodes 5",
		"kwatch_delivery_retries_total 6",
		"kwatch_persistence_migration_errors_total 7",
		"kwatch_shutdown_timeouts_total 8",
		"kwatch_source_unavailable_total 9",
		"kwatch_leadership_acquisitions_total 10",
		"kwatch_leadership_losses_total 11",
		"kwatch_leader_takeovers_total 12",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics output is missing %q", metric)
		}
	}
}
