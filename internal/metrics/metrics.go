package metrics

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is the application metrics state. Atomic fields keep metric writes
// cheap for hot paths; the Prometheus collector below owns exposition and
// validates the metric contract through the standard client library.
type Registry struct {
	IncidentsCreate            atomic.Int64
	IncidentsUpdate            atomic.Int64
	IncidentsResolved          atomic.Int64
	IncidentsGrouped           atomic.Int64
	NotificationsTotal         atomic.Int64
	NotificationsDropped       atomic.Int64
	BaselineSize               atomic.Int64
	ActiveIncidents            atomic.Int64
	GraphNodes                 atomic.Int64
	GraphEdges                 atomic.Int64
	APIServerProbeErrors       atomic.Int64
	APIServerLatencyMs         atomic.Int64
	ControlPlaneProbeErrors    atomic.Int64
	InformerWatchErrors        atomic.Int64
	InformerEvents             atomic.Int64
	InformerHandlerPanics      atomic.Int64
	QueueDepth                 atomic.Int64
	ProcessingLatencyMs        atomic.Int64
	GraphRebuilds              atomic.Int64
	GraphRebuildLatencyMs      atomic.Int64
	DeliveryRetries            atomic.Int64
	DeliveryTerminalErrors     atomic.Int64
	DeliveryDeadLetters        atomic.Int64
	DeliveryQueueSaturated     atomic.Int64
	PersistenceMigrations      atomic.Int64
	PersistenceMigrationErr    atomic.Int64
	OptionalAPIUnavailable     atomic.Int64
	WatcherSyncs               atomic.Int64
	WatcherSyncFailures        atomic.Int64
	ComponentDegradations      atomic.Int64
	ComponentStalls            atomic.Int64
	ComponentUnexpectedStops   atomic.Int64
	ShutdownTimeouts           atomic.Int64
	SourceUnavailable          atomic.Int64
	LeadershipAcquisitions     atomic.Int64
	LeadershipLosses           atomic.Int64
	LeaderTakeovers            atomic.Int64
	TelemetryAttempts          atomic.Int64
	TelemetrySuccesses         atomic.Int64
	TelemetryRetries           atomic.Int64
	TelemetryFailures          [5]atomic.Int64
	PersistenceRetries         atomic.Int64
	PersistenceCompactions     atomic.Int64
	PersistenceOmitted         atomic.Int64
	PersistencePayloadBytes    atomic.Int64
	PersistenceLastSuccess     atomic.Int64
	DuplicateTransitions       atomic.Int64
	GroupSize                  atomic.Int64
	GroupedChildCount          atomic.Int64
	RootCauseSuppressions      atomic.Int64
	RenderedDetailsOmitted     atomic.Int64
	RedactedValues             atomic.Int64
	StartupSummariesSuppressed atomic.Int64

	registryOnce sync.Once
	registry     *prometheus.Registry
}

var defaultRegistry = &Registry{}

// DefaultRegistry returns the process-wide metrics registry through the
// package boundary used by internal components.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

func (r *Registry) initPrometheus() {
	r.registryOnce.Do(func() {
		r.registry = prometheus.NewRegistry()
		r.registry.MustRegister(r)
	})
}

// Handler exposes the registry using the standard Prometheus HTTP handler.
// The method check preserves kwatch's existing endpoint contract.
func (r *Registry) Handler() http.Handler {
	r.initPrometheus()
	handler := promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		handler.ServeHTTP(w, req)
	})
}

var metricDescs = []*prometheus.Desc{
	prometheus.NewDesc("kwatch_incidents_total",
		"Total incidents by action", []string{"action"}, nil),
	prometheus.NewDesc("kwatch_apiserver_probe_errors_total",
		"API server health probe failures", nil, nil),
	prometheus.NewDesc("kwatch_apiserver_latency_milliseconds",
		"Latest API server readyz latency", nil, nil),
	prometheus.NewDesc("kwatch_controlplane_probe_errors_total",
		"Control-plane component probe failures", nil, nil),
	prometheus.NewDesc("kwatch_informer_watch_errors_total",
		"Informer watch interruptions", nil, nil),
	prometheus.NewDesc("kwatch_informer_events_total",
		"Informer events received by kwatch", nil, nil),
	prometheus.NewDesc("kwatch_informer_handler_panics_total",
		"Informer event handler panics recovered by kwatch", nil, nil),
	prometheus.NewDesc("kwatch_queue_depth",
		"Current aggregate workqueue depth", nil, nil),
	prometheus.NewDesc("kwatch_processing_latency_milliseconds",
		"Latest work item processing latency", nil, nil),
	prometheus.NewDesc("kwatch_graph_rebuilds_total",
		"Dependency graph rebuild attempts", nil, nil),
	prometheus.NewDesc("kwatch_graph_rebuild_latency_milliseconds",
		"Latest dependency graph rebuild latency", nil, nil),
	prometheus.NewDesc("kwatch_notifications_total",
		"Total notification attempts", nil, nil),
	prometheus.NewDesc("kwatch_notifications_dropped_total",
		"Notifications dropped (channel full)", nil, nil),
	prometheus.NewDesc("kwatch_incidents_active",
		"Currently active incidents", nil, nil),
	prometheus.NewDesc("kwatch_baseline_size",
		"Baseline entries (seen pods)", nil, nil),
	prometheus.NewDesc("kwatch_graph_nodes",
		"Resources in the dependency graph", nil, nil),
	prometheus.NewDesc("kwatch_graph_edges",
		"Relationships in the dependency graph", nil, nil),
	prometheus.NewDesc("kwatch_delivery_retries_total",
		"Delivery retry attempts", nil, nil),
	prometheus.NewDesc("kwatch_delivery_terminal_errors_total",
		"Terminal delivery failures", nil, nil),
	prometheus.NewDesc("kwatch_delivery_dead_letters_total",
		"Delivery dead-letter entries", nil, nil),
	prometheus.NewDesc("kwatch_delivery_queue_saturated_total",
		"Delivery queue saturation events", nil, nil),
	prometheus.NewDesc("kwatch_persistence_migrations_total",
		"Persistence migrations completed or checked", nil, nil),
	prometheus.NewDesc("kwatch_persistence_migration_errors_total",
		"Persistence migration failures", nil, nil),
	prometheus.NewDesc("kwatch_optional_api_unavailable_total",
		"Optional APIs unavailable during watcher setup", nil, nil),
	prometheus.NewDesc("kwatch_watcher_syncs_total",
		"Dynamic watcher cache synchronization attempts", nil, nil),
	prometheus.NewDesc("kwatch_watcher_sync_failures_total",
		"Dynamic watcher cache synchronization failures", nil, nil),
	prometheus.NewDesc("kwatch_component_degradations_total",
		"Optional component degradation events", nil, nil),
	prometheus.NewDesc("kwatch_component_stalls_total",
		"Required component stall detections", nil, nil),
	prometheus.NewDesc("kwatch_component_unexpected_stops_total",
		"Unexpected component stops", nil, nil),
	prometheus.NewDesc("kwatch_shutdown_timeouts_total",
		"Component shutdown timeouts", nil, nil),
	prometheus.NewDesc("kwatch_source_unavailable_total",
		"Required monitor source capabilities unavailable", nil, nil),
	prometheus.NewDesc("kwatch_leadership_acquisitions_total",
		"Leader election acquisitions", nil, nil),
	prometheus.NewDesc("kwatch_leadership_losses_total",
		"Leader election losses", nil, nil),
	prometheus.NewDesc("kwatch_leader_takeovers_total",
		"Leader election takeovers", nil, nil),
	prometheus.NewDesc("kwatch_telemetry_attempts_total",
		"Adoption telemetry HTTP attempts", nil, nil),
	prometheus.NewDesc("kwatch_telemetry_successes_total",
		"Successful adoption telemetry reports", nil, nil),
	prometheus.NewDesc("kwatch_telemetry_failures_total",
		"Failed adoption telemetry operations", []string{"reason"}, nil),
	prometheus.NewDesc("kwatch_telemetry_retries_total",
		"Adoption telemetry retries", nil, nil),
	prometheus.NewDesc("kwatch_persistence_retries_total",
		"Persistence write retries", nil, nil),
	prometheus.NewDesc("kwatch_persistence_compactions_total",
		"Persistence payload compactions", nil, nil),
	prometheus.NewDesc("kwatch_persistence_omitted_total",
		"Persistence entries omitted during compaction", nil, nil),
	prometheus.NewDesc("kwatch_persistence_payload_bytes",
		"Latest persistence payload size", nil, nil),
	prometheus.NewDesc("kwatch_persistence_last_success_timestamp_seconds",
		"Unix timestamp of the last successful persistence write", nil, nil),
	prometheus.NewDesc("kwatch_lifecycle_duplicate_transitions_total",
		"Duplicate lifecycle transitions suppressed", nil, nil),
	prometheus.NewDesc("kwatch_group_size",
		"Latest smart-group member count", nil, nil),
	prometheus.NewDesc("kwatch_grouped_children_total",
		"Grouped child incidents", nil, nil),
	prometheus.NewDesc("kwatch_root_cause_suppressions_total",
		"Child incidents suppressed by root causes", nil, nil),
	prometheus.NewDesc("kwatch_rendered_details_omitted_total",
		"Provider detail sections omitted by bounds", nil, nil),
	prometheus.NewDesc("kwatch_redacted_values_total",
		"Sensitive values redacted before rendering", nil, nil),
	prometheus.NewDesc("kwatch_startup_summaries_suppressed_total",
		"Startup summaries suppressed as routine restarts", nil, nil),
}

var telemetryFailureReasons = [...]string{
	"state_read", "invalid_identity", "network", "http_status", "state_write",
}

// IncTelemetryFailure records one failure using a bounded reason label.
func (r *Registry) IncTelemetryFailure(reason string) {
	for i, allowed := range telemetryFailureReasons {
		if reason == allowed {
			r.TelemetryFailures[i].Add(1)
			return
		}
	}
	r.TelemetryFailures[2].Add(1)
}

// Describe implements prometheus.Collector.
func (r *Registry) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range metricDescs {
		ch <- desc
	}
}

// Collect implements prometheus.Collector. Labels are a fixed, bounded set;
// resource names, incident IDs, and provider URLs never become labels.
func (r *Registry) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(
		metricDescs[0], prometheus.CounterValue,
		float64(r.IncidentsCreate.Load()), "create",
	)
	ch <- prometheus.MustNewConstMetric(
		metricDescs[0], prometheus.CounterValue,
		float64(r.IncidentsUpdate.Load()), "update",
	)
	ch <- prometheus.MustNewConstMetric(
		metricDescs[0], prometheus.CounterValue,
		float64(r.IncidentsResolved.Load()), "resolved",
	)
	ch <- prometheus.MustNewConstMetric(
		metricDescs[0], prometheus.CounterValue,
		float64(r.IncidentsGrouped.Load()), "grouped",
	)

	r.collectCounter(ch, 1, r.APIServerProbeErrors.Load())
	r.collectGauge(ch, 2, r.APIServerLatencyMs.Load())
	r.collectCounter(ch, 3, r.ControlPlaneProbeErrors.Load())
	r.collectCounter(ch, 4, r.InformerWatchErrors.Load())
	r.collectCounter(ch, 5, r.InformerEvents.Load())
	r.collectCounter(ch, 6, r.InformerHandlerPanics.Load())
	r.collectGauge(ch, 7, r.QueueDepth.Load())
	r.collectGauge(ch, 8, r.ProcessingLatencyMs.Load())
	r.collectCounter(ch, 9, r.GraphRebuilds.Load())
	r.collectGauge(ch, 10, r.GraphRebuildLatencyMs.Load())
	r.collectCounter(ch, 11, r.NotificationsTotal.Load())
	r.collectCounter(ch, 12, r.NotificationsDropped.Load())
	r.collectGauge(ch, 13, r.ActiveIncidents.Load())
	r.collectGauge(ch, 14, r.BaselineSize.Load())
	r.collectGauge(ch, 15, r.GraphNodes.Load())
	r.collectGauge(ch, 16, r.GraphEdges.Load())
	r.collectCounter(ch, 17, r.DeliveryRetries.Load())
	r.collectCounter(ch, 18, r.DeliveryTerminalErrors.Load())
	r.collectCounter(ch, 19, r.DeliveryDeadLetters.Load())
	r.collectCounter(ch, 20, r.DeliveryQueueSaturated.Load())
	r.collectCounter(ch, 21, r.PersistenceMigrations.Load())
	r.collectCounter(ch, 22, r.PersistenceMigrationErr.Load())
	r.collectCounter(ch, 23, r.OptionalAPIUnavailable.Load())
	r.collectCounter(ch, 24, r.WatcherSyncs.Load())
	r.collectCounter(ch, 25, r.WatcherSyncFailures.Load())
	r.collectCounter(ch, 26, r.ComponentDegradations.Load())
	r.collectCounter(ch, 27, r.ComponentStalls.Load())
	r.collectCounter(ch, 28, r.ComponentUnexpectedStops.Load())
	r.collectCounter(ch, 29, r.ShutdownTimeouts.Load())
	r.collectCounter(ch, 30, r.SourceUnavailable.Load())
	r.collectCounter(ch, 31, r.LeadershipAcquisitions.Load())
	r.collectCounter(ch, 32, r.LeadershipLosses.Load())
	r.collectCounter(ch, 33, r.LeaderTakeovers.Load())
	r.collectCounter(ch, 34, r.TelemetryAttempts.Load())
	r.collectCounter(ch, 35, r.TelemetrySuccesses.Load())
	for i, reason := range telemetryFailureReasons {
		ch <- prometheus.MustNewConstMetric(
			metricDescs[36], prometheus.CounterValue,
			float64(r.TelemetryFailures[i].Load()), reason,
		)
	}
	r.collectCounter(ch, 37, r.TelemetryRetries.Load())
	r.collectCounter(ch, 38, r.PersistenceRetries.Load())
	r.collectCounter(ch, 39, r.PersistenceCompactions.Load())
	r.collectCounter(ch, 40, r.PersistenceOmitted.Load())
	r.collectGauge(ch, 41, r.PersistencePayloadBytes.Load())
	r.collectGauge(ch, 42, r.PersistenceLastSuccess.Load())
	r.collectCounter(ch, 43, r.DuplicateTransitions.Load())
	r.collectGauge(ch, 44, r.GroupSize.Load())
	r.collectCounter(ch, 45, r.GroupedChildCount.Load())
	r.collectCounter(ch, 46, r.RootCauseSuppressions.Load())
	r.collectCounter(ch, 47, r.RenderedDetailsOmitted.Load())
	r.collectCounter(ch, 48, r.RedactedValues.Load())
	r.collectCounter(ch, 49, r.StartupSummariesSuppressed.Load())
}

func (r *Registry) collectCounter(
	ch chan<- prometheus.Metric, index int, value int64,
) {
	ch <- prometheus.MustNewConstMetric(
		metricDescs[index], prometheus.CounterValue, float64(value),
	)
}

func (r *Registry) collectGauge(
	ch chan<- prometheus.Metric, index int, value int64,
) {
	ch <- prometheus.MustNewConstMetric(
		metricDescs[index], prometheus.GaugeValue, float64(value),
	)
}
