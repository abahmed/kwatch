package metrics

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is the application metrics state. Atomic fields keep metric writes
// cheap for hot paths; the Prometheus collector below owns exposition and
// validates the metric contract through the standard client library. Every
// field has a production writer; TestRegistryFieldsHaveWriters enforces it.
type Registry struct {
	IncidentActions          [3]atomic.Int64
	IncidentsOpen            atomic.Int64
	NotificationsTotal       atomic.Int64
	NotificationsDropped     atomic.Int64
	InformerHandlerPanics    atomic.Int64
	DeliveryRetries          atomic.Int64
	DeliveryTerminalErrors   atomic.Int64
	DeliveryDeadLetters      atomic.Int64
	DeliveryResolvesLost     atomic.Int64
	DeliveryQueueSaturated   atomic.Int64
	DeliveryPendingDropped   atomic.Int64
	DeliveryDigestSkipped    atomic.Int64
	OutboxDropped            atomic.Int64
	OutboxWriteFailures      atomic.Int64
	HeartbeatFailures        atomic.Int64
	KubeletStatsFailures     atomic.Int64
	OptionalAPIUnavailable   atomic.Int64
	WatcherSyncs             atomic.Int64
	WatcherSyncFailures      atomic.Int64
	ComponentDegradations    atomic.Int64
	ComponentStalls          atomic.Int64
	ComponentUnexpectedStops atomic.Int64
	ShutdownTimeouts         atomic.Int64
	SourceUnavailable        atomic.Int64
	LeadershipAcquisitions   atomic.Int64
	LeadershipLosses         atomic.Int64
	LeaderTakeovers          atomic.Int64
	TelemetryAttempts        atomic.Int64
	TelemetrySuccesses       atomic.Int64
	TelemetryRetries         atomic.Int64
	TelemetryFailures        [5]atomic.Int64
	RenderedDetailsOmitted   atomic.Int64
	RedactedValues           atomic.Int64
	StorageResets            [2]atomic.Int64
	StorageCorruptRecords    atomic.Int64
	StorageExpired           atomic.Int64
	StorageEvicted           atomic.Int64
	StorageWriteFailures     atomic.Int64
	StorageRewrites          atomic.Int64
	StorageFileBytes         atomic.Int64
	StorageFreeBytes         atomic.Int64
	Investigations           [4]atomic.Int64
	AuditDropped             atomic.Int64
	DecisionLag              LagHistogram
	// Delivery holds the per-provider and outbox delivery metrics.
	Delivery DeliveryMetrics

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
		r.registry.MustRegister(
			r,
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(
				collectors.ProcessCollectorOpts{},
			),
		)
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

// scalarMetric is a metric backed by exactly one Registry field.
type scalarMetric struct {
	desc  *prometheus.Desc
	gauge bool
	load  func(*Registry) int64
}

func counter(name, help string, load func(*Registry) int64) scalarMetric {
	return scalarMetric{
		desc: prometheus.NewDesc(name, help, nil, nil), load: load,
	}
}

func gauge(name, help string, load func(*Registry) int64) scalarMetric {
	return scalarMetric{
		desc:  prometheus.NewDesc(name, help, nil, nil),
		gauge: true, load: load,
	}
}

var (
	incidentsDesc = prometheus.NewDesc("kwatch_incidents_total",
		"Incident lifecycle decisions by action", []string{"action"}, nil)
	telemetryFailuresDesc = prometheus.NewDesc(
		"kwatch_telemetry_failures_total",
		"Failed adoption telemetry operations", []string{"reason"}, nil)

	storageResetsDesc = prometheus.NewDesc(
		"kwatch_storage_resets_total",
		"State files deleted and recreated at open", []string{"reason"}, nil)

	investigationsDesc = prometheus.NewDesc(
		"kwatch_investigations_total",
		"Announcement investigations by result", []string{"result"}, nil)

	investigationResults = [...]string{"done", "skipped", "late", "timeout"}

	storageResetReasons = [...]string{"schema_mismatch", "unreadable"}

	incidentActions = [...]string{"announce", "update", "resolve"}

	telemetryFailureReasons = [...]string{
		"state_read", "invalid_identity", "network", "http_status",
		"state_write",
	}
)

var scalarMetrics = []scalarMetric{
	gauge("kwatch_incidents_open", "Incidents that are not yet resolved",
		func(r *Registry) int64 { return r.IncidentsOpen.Load() }),
	counter("kwatch_delivery_notifications_total",
		"Total notification attempts",
		func(r *Registry) int64 { return r.NotificationsTotal.Load() }),
	counter("kwatch_delivery_dropped_total",
		"Notifications no provider accepted (dead-lettered or dropped)",
		func(r *Registry) int64 { return r.NotificationsDropped.Load() }),
	counter("kwatch_delivery_retries_total", "Delivery retry attempts",
		func(r *Registry) int64 { return r.DeliveryRetries.Load() }),
	counter("kwatch_delivery_terminal_errors_total",
		"Terminal delivery failures",
		func(r *Registry) int64 { return r.DeliveryTerminalErrors.Load() }),
	counter("kwatch_delivery_dead_letters_total",
		"Delivery dead-letter entries",
		func(r *Registry) int64 { return r.DeliveryDeadLetters.Load() }),
	counter("kwatch_delivery_resolves_lost_total",
		"Resolves given up on, which can leave an alert open",
		func(r *Registry) int64 { return r.DeliveryResolvesLost.Load() }),
	counter("kwatch_delivery_queue_saturated_total",
		"Delivery queue saturation events",
		func(r *Registry) int64 { return r.DeliveryQueueSaturated.Load() }),
	counter("kwatch_delivery_pending_dropped_total",
		"Notifications dropped before delivery started",
		func(r *Registry) int64 { return r.DeliveryPendingDropped.Load() }),
	counter("kwatch_delivery_digest_skipped_total",
		"Overflow digests not sent because the provider skips plain messages",
		func(r *Registry) int64 { return r.DeliveryDigestSkipped.Load() }),
	counter("kwatch_delivery_outbox_dropped_total",
		"Persisted delivery jobs dropped by the outbox size or age bound",
		func(r *Registry) int64 { return r.OutboxDropped.Load() }),
	counter("kwatch_delivery_outbox_write_failures_total",
		"Failed writes of the persisted delivery outbox",
		func(r *Registry) int64 { return r.OutboxWriteFailures.Load() }),
	counter("kwatch_heartbeat_failures_total",
		"Heartbeat pings that failed or were rejected",
		func(r *Registry) int64 { return r.HeartbeatFailures.Load() }),
	counter("kwatch_kubelet_stats_failures_total",
		"Kubelet stats summary reads that failed, counted per node",
		func(r *Registry) int64 { return r.KubeletStatsFailures.Load() }),
	counter("kwatch_informer_handler_panics_total",
		"Informer event handler panics recovered by kwatch",
		func(r *Registry) int64 { return r.InformerHandlerPanics.Load() }),
	counter("kwatch_optional_api_unavailable_total",
		"Optional APIs unavailable during watcher setup",
		func(r *Registry) int64 { return r.OptionalAPIUnavailable.Load() }),
	counter("kwatch_watcher_syncs_total",
		"Dynamic watcher cache synchronization attempts",
		func(r *Registry) int64 { return r.WatcherSyncs.Load() }),
	counter("kwatch_watcher_sync_failures_total",
		"Dynamic watcher cache synchronization failures",
		func(r *Registry) int64 { return r.WatcherSyncFailures.Load() }),
	counter("kwatch_component_degradations_total",
		"Optional component degradation events",
		func(r *Registry) int64 { return r.ComponentDegradations.Load() }),
	counter("kwatch_component_stalls_total",
		"Required component stall detections",
		func(r *Registry) int64 { return r.ComponentStalls.Load() }),
	counter("kwatch_component_unexpected_stops_total",
		"Unexpected component stops",
		func(r *Registry) int64 { return r.ComponentUnexpectedStops.Load() }),
	counter("kwatch_shutdown_timeouts_total", "Component shutdown timeouts",
		func(r *Registry) int64 { return r.ShutdownTimeouts.Load() }),
	counter("kwatch_source_unavailable_total",
		"Required monitor source capabilities unavailable",
		func(r *Registry) int64 { return r.SourceUnavailable.Load() }),
	counter("kwatch_leadership_acquisitions_total",
		"Leader election acquisitions",
		func(r *Registry) int64 { return r.LeadershipAcquisitions.Load() }),
	counter("kwatch_leadership_losses_total", "Leader election losses",
		func(r *Registry) int64 { return r.LeadershipLosses.Load() }),
	counter("kwatch_leader_takeovers_total", "Leader election takeovers",
		func(r *Registry) int64 { return r.LeaderTakeovers.Load() }),
	counter("kwatch_telemetry_attempts_total",
		"Adoption telemetry HTTP attempts",
		func(r *Registry) int64 { return r.TelemetryAttempts.Load() }),
	counter("kwatch_telemetry_successes_total",
		"Successful adoption telemetry reports",
		func(r *Registry) int64 { return r.TelemetrySuccesses.Load() }),
	counter("kwatch_telemetry_retries_total", "Adoption telemetry retries",
		func(r *Registry) int64 { return r.TelemetryRetries.Load() }),
	counter("kwatch_rendered_details_omitted_total",
		"Provider detail sections omitted by bounds",
		func(r *Registry) int64 { return r.RenderedDetailsOmitted.Load() }),
	counter("kwatch_redacted_values_total",
		"Sensitive values redacted before rendering",
		func(r *Registry) int64 { return r.RedactedValues.Load() }),
	counter("kwatch_storage_corrupt_records_total",
		"Stored values skipped because they did not decode",
		func(r *Registry) int64 { return r.StorageCorruptRecords.Load() }),
	counter("kwatch_storage_expired_total",
		"Stored entries deleted by retention",
		func(r *Registry) int64 { return r.StorageExpired.Load() }),
	counter("kwatch_storage_evicted_total",
		"Stored entries deleted to meet the size cap",
		func(r *Registry) int64 { return r.StorageEvicted.Load() }),
	counter("kwatch_storage_write_failures_total",
		"Pipeline storage batches with at least one failed write",
		func(r *Registry) int64 { return r.StorageWriteFailures.Load() }),
	counter("kwatch_storage_rewrites_total",
		"Times the state file was rewritten while running to shrink it",
		func(r *Registry) int64 { return r.StorageRewrites.Load() }),
	gauge("kwatch_storage_file_bytes",
		"Size of the state file after the last compactor pass",
		func(r *Registry) int64 { return r.StorageFileBytes.Load() }),
	gauge("kwatch_storage_free_bytes",
		"Free pages inside the state file after the last compactor pass",
		func(r *Registry) int64 { return r.StorageFreeBytes.Load() }),
	counter("kwatch_audit_dropped_total",
		"Audit entries dropped because the audit queue was full",
		func(r *Registry) int64 { return r.AuditDropped.Load() }),
}

// IncIncident records one lifecycle decision using a bounded action label.
// Unknown actions are ignored so the label set cannot grow.
func (r *Registry) IncIncident(action string) {
	for i, allowed := range incidentActions {
		if action == allowed {
			r.IncidentActions[i].Add(1)
			return
		}
	}
}

// IncInvestigation records one investigation outcome using a bounded
// result label. Unknown results are ignored.
func (r *Registry) IncInvestigation(result string) {
	for i, allowed := range investigationResults {
		if result == allowed {
			r.Investigations[i].Add(1)
			return
		}
	}
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

// IncStorageReset records one state file reset using a bounded reason
// label; an unknown reason counts as unreadable.
func (r *Registry) IncStorageReset(reason string) {
	for i, allowed := range storageResetReasons {
		if reason == allowed {
			r.StorageResets[i].Add(1)
			return
		}
	}
	r.StorageResets[1].Add(1)
}

// Describe implements prometheus.Collector.
func (r *Registry) Describe(ch chan<- *prometheus.Desc) {
	ch <- incidentsDesc
	ch <- telemetryFailuresDesc
	ch <- storageResetsDesc
	ch <- investigationsDesc
	ch <- decisionLagDesc
	r.describeDelivery(ch)
	for _, m := range scalarMetrics {
		ch <- m.desc
	}
}

// Collect implements prometheus.Collector. Labels are a fixed, bounded set;
// resource names, incident IDs, and provider URLs never become labels.
func (r *Registry) Collect(ch chan<- prometheus.Metric) {
	for i, action := range incidentActions {
		ch <- prometheus.MustNewConstMetric(
			incidentsDesc, prometheus.CounterValue,
			float64(r.IncidentActions[i].Load()), action,
		)
	}
	for i, reason := range telemetryFailureReasons {
		ch <- prometheus.MustNewConstMetric(
			telemetryFailuresDesc, prometheus.CounterValue,
			float64(r.TelemetryFailures[i].Load()), reason,
		)
	}
	for i, reason := range storageResetReasons {
		ch <- prometheus.MustNewConstMetric(
			storageResetsDesc, prometheus.CounterValue,
			float64(r.StorageResets[i].Load()), reason,
		)
	}
	for i, result := range investigationResults {
		ch <- prometheus.MustNewConstMetric(
			investigationsDesc, prometheus.CounterValue,
			float64(r.Investigations[i].Load()), result,
		)
	}
	ch <- r.DecisionLag.metric()
	r.collectDelivery(ch)
	for _, m := range scalarMetrics {
		kind := prometheus.CounterValue
		if m.gauge {
			kind = prometheus.GaugeValue
		}
		ch <- prometheus.MustNewConstMetric(
			m.desc, kind, float64(m.load(r)),
		)
	}
}
