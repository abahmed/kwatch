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
	IncidentsCreate         atomic.Int64
	IncidentsUpdate         atomic.Int64
	IncidentsResolved       atomic.Int64
	IncidentsGrouped        atomic.Int64
	NotificationsTotal      atomic.Int64
	NotificationsDropped    atomic.Int64
	BaselineSize            atomic.Int64
	ActiveIncidents         atomic.Int64
	GraphNodes              atomic.Int64
	GraphEdges              atomic.Int64
	APIServerProbeErrors    atomic.Int64
	APIServerLatencyMs      atomic.Int64
	ControlPlaneProbeErrors atomic.Int64
	InformerWatchErrors     atomic.Int64
	InformerEvents          atomic.Int64
	QueueDepth              atomic.Int64
	ProcessingLatencyMs     atomic.Int64
	GraphRebuilds           atomic.Int64
	GraphRebuildLatencyMs   atomic.Int64
	DeliveryRetries         atomic.Int64
	DeliveryTerminalErrors  atomic.Int64
	DeliveryDeadLetters     atomic.Int64
	DeliveryQueueSaturated  atomic.Int64
	PersistenceMigrations   atomic.Int64
	PersistenceMigrationErr atomic.Int64
	OptionalAPIUnavailable  atomic.Int64
	WatcherSyncs            atomic.Int64
	WatcherSyncFailures     atomic.Int64
	ComponentDegradations   atomic.Int64
	ShutdownTimeouts        atomic.Int64
	SourceUnavailable       atomic.Int64

	registryOnce sync.Once
	registry     *prometheus.Registry
}

var defaultRegistry = &Registry{}

// Default is retained as a compatibility alias. New internal code must use
// DefaultRegistry so the process-wide registry remains behind one boundary.
var Default = defaultRegistry

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
	prometheus.NewDesc("kwatch_shutdown_timeouts_total",
		"Component shutdown timeouts", nil, nil),
	prometheus.NewDesc("kwatch_source_unavailable_total",
		"Required monitor source capabilities unavailable", nil, nil),
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
	r.collectGauge(ch, 6, r.QueueDepth.Load())
	r.collectGauge(ch, 7, r.ProcessingLatencyMs.Load())
	r.collectCounter(ch, 8, r.GraphRebuilds.Load())
	r.collectGauge(ch, 9, r.GraphRebuildLatencyMs.Load())
	r.collectCounter(ch, 10, r.NotificationsTotal.Load())
	r.collectCounter(ch, 11, r.NotificationsDropped.Load())
	r.collectGauge(ch, 12, r.ActiveIncidents.Load())
	r.collectGauge(ch, 13, r.BaselineSize.Load())
	r.collectGauge(ch, 14, r.GraphNodes.Load())
	r.collectGauge(ch, 15, r.GraphEdges.Load())
	r.collectCounter(ch, 16, r.DeliveryRetries.Load())
	r.collectCounter(ch, 17, r.DeliveryTerminalErrors.Load())
	r.collectCounter(ch, 18, r.DeliveryDeadLetters.Load())
	r.collectCounter(ch, 19, r.DeliveryQueueSaturated.Load())
	r.collectCounter(ch, 20, r.PersistenceMigrations.Load())
	r.collectCounter(ch, 21, r.PersistenceMigrationErr.Load())
	r.collectCounter(ch, 22, r.OptionalAPIUnavailable.Load())
	r.collectCounter(ch, 23, r.WatcherSyncs.Load())
	r.collectCounter(ch, 24, r.WatcherSyncFailures.Load())
	r.collectCounter(ch, 25, r.ComponentDegradations.Load())
	r.collectCounter(ch, 26, r.ShutdownTimeouts.Load())
	r.collectCounter(ch, 27, r.SourceUnavailable.Load())
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
