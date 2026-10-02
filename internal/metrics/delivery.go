package metrics

import (
	"sort"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// DeliveryMetrics are the delivery gauges and counters that are not plain
// process totals: the per-provider queue depth and the outbox depth.
type DeliveryMetrics struct {
	// OutboxDepth is the number of persisted jobs not yet settled.
	OutboxDepth atomic.Int64
	// Deferred counts jobs left in the outbox at shutdown for the next
	// session to send; they are not dead letters.
	Deferred atomic.Int64
	// BudgetFolded counts notifications folded into a digest because the
	// provider's hourly budget was spent.
	BudgetFolded atomic.Int64
	// TrackerUntracked counts issue-tracker creates that succeeded but
	// returned an issue reference kwatch could not read.
	TrackerUntracked atomic.Int64

	queueMu    sync.Mutex
	queueDepth map[string]int64
}

// ConfigureQueueProviders replaces the provider label set of the queue
// depth gauge with the configured providers, so the label cardinality is
// bounded by the configuration and a removed provider stops reporting.
func (d *DeliveryMetrics) ConfigureQueueProviders(names []string) {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()
	d.queueDepth = make(map[string]int64, len(names))
	for _, name := range names {
		d.queueDepth[name] = 0
	}
}

// SetQueueDepth records one provider's queued jobs. A provider that is not
// configured is ignored, so the label set cannot grow.
func (d *DeliveryMetrics) SetQueueDepth(provider string, depth int) {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()
	if _, ok := d.queueDepth[provider]; ok {
		d.queueDepth[provider] = int64(depth)
	}
}

// QueueDepth returns one provider's last recorded depth.
func (d *DeliveryMetrics) QueueDepth(provider string) int64 {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()
	return d.queueDepth[provider]
}

var (
	queueDepthDesc = prometheus.NewDesc("kwatch_delivery_queue_depth",
		"Jobs waiting in each configured provider's delivery queue",
		[]string{"provider"}, nil)

	deliveryScalars = []scalarMetric{
		gauge("kwatch_delivery_outbox_depth",
			"Persisted delivery jobs not yet delivered or given up",
			func(r *Registry) int64 { return r.Delivery.OutboxDepth.Load() }),
		counter("kwatch_delivery_deferred_total",
			"Jobs left in the outbox at shutdown for the next session",
			func(r *Registry) int64 { return r.Delivery.Deferred.Load() }),
		counter("kwatch_delivery_budget_folded_total",
			"Notifications folded into a digest by the hourly budget",
			func(r *Registry) int64 { return r.Delivery.BudgetFolded.Load() }),
		counter("kwatch_tracker_untracked_total",
			"Issue creates whose response did not name the new issue",
			func(r *Registry) int64 {
				return r.Delivery.TrackerUntracked.Load()
			}),
	}
)

func (r *Registry) describeDelivery(ch chan<- *prometheus.Desc) {
	ch <- queueDepthDesc
	for _, m := range deliveryScalars {
		ch <- m.desc
	}
}

// collectDelivery always reports at least one queue depth series, so the
// metric family exists before any provider is configured.
func (r *Registry) collectDelivery(ch chan<- prometheus.Metric) {
	r.Delivery.queueMu.Lock()
	names := make([]string, 0, len(r.Delivery.queueDepth))
	for name := range r.Delivery.queueDepth {
		names = append(names, name)
	}
	sort.Strings(names)
	depths := make([]int64, len(names))
	for i, name := range names {
		depths[i] = r.Delivery.queueDepth[name]
	}
	r.Delivery.queueMu.Unlock()
	if len(names) == 0 {
		names, depths = []string{"none"}, []int64{0}
	}
	for i, name := range names {
		ch <- prometheus.MustNewConstMetric(queueDepthDesc,
			prometheus.GaugeValue, float64(depths[i]), name)
	}
	for _, m := range deliveryScalars {
		kind := prometheus.CounterValue
		if m.gauge {
			kind = prometheus.GaugeValue
		}
		ch <- prometheus.MustNewConstMetric(m.desc, kind, float64(m.load(r)))
	}
}
