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
	r.InformerHandlerPanics.Store(13)
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
		"kwatch_informer_handler_panics_total 13",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics output is missing %q", metric)
		}
	}
}

func TestHandlerExposesReliabilityMetrics(t *testing.T) {
	r := &Registry{}
	r.PersistenceRetries.Store(2)
	r.PersistenceCompactions.Store(3)
	r.PersistenceOmitted.Store(4)
	r.PersistencePayloadBytes.Store(5)
	r.PersistenceLastSuccess.Store(6)
	r.DuplicateTransitions.Store(7)
	r.GroupSize.Store(8)
	r.GroupedChildCount.Store(9)
	r.RootCauseSuppressions.Store(10)
	r.RenderedDetailsOmitted.Store(11)
	r.RedactedValues.Store(12)
	r.StartupSummariesSuppressed.Store(13)
	r.InsightAnalyses.Store(14)
	r.InsightConfirmed.Store(15)
	r.InsightLikely.Store(16)
	r.InsightUnknown.Store(17)
	r.InsightReevaluations.Store(18)
	r.InsightRolloutSuppressions.Store(19)

	rr := httptest.NewRecorder()
	r.Handler().ServeHTTP(
		rr, httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	body := rr.Body.String()
	for _, metric := range []string{
		"kwatch_persistence_retries_total 2",
		"kwatch_persistence_compactions_total 3",
		"kwatch_persistence_omitted_total 4",
		"kwatch_persistence_payload_bytes 5",
		"kwatch_persistence_last_success_timestamp_seconds 6",
		"kwatch_lifecycle_duplicate_transitions_total 7",
		"kwatch_group_size 8",
		"kwatch_grouped_children_total 9",
		"kwatch_root_cause_suppressions_total 10",
		"kwatch_rendered_details_omitted_total 11",
		"kwatch_redacted_values_total 12",
		"kwatch_startup_summaries_suppressed_total 13",
		"kwatch_insight_analyses_total 14",
		"kwatch_insight_causes_total{state=\"confirmed\"} 15",
		"kwatch_insight_causes_total{state=\"likely\"} 16",
		"kwatch_insight_causes_total{state=\"unknown\"} 17",
		"kwatch_insight_reevaluations_total 18",
		"kwatch_insight_rollout_suppressions_total 19",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics output is missing %q", metric)
		}
	}
}
