package inventory

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Metric names one rolling workload statistic. Names are stable: they
// are persisted.
type Metric string

// Baseline metrics.
const (
	// MetricRestartsPerHour is container restarts per hour, per workload.
	MetricRestartsPerHour Metric = "restarts_per_hour"
	// MetricReadySeconds is how long a new pod takes to become ready.
	MetricReadySeconds Metric = "ready_seconds"
	// MetricPendingSeconds is how long a new pod waits before it starts.
	MetricPendingSeconds Metric = "pending_seconds"
	// MetricJobSeconds is how long a Job runs until it completes.
	MetricJobSeconds Metric = "job_seconds"
	// MetricMemoryPeak is the largest container memory (resident set,
	// else working set) of the workload in an hour, in bytes.
	MetricMemoryPeak Metric = "memory_peak_bytes"
)

// warningMetricPrefix starts the name of a per-reason Warning event
// rate; WarningMetric builds the whole name.
const warningMetricPrefix = "warnings_per_hour/"

// WarningMetric is the events per hour of one Warning reason.
func WarningMetric(reason string) Metric {
	return Metric(warningMetricPrefix + reason)
}

// Sketch sizes and smoothing.
const (
	// sketchSize is how many recent samples an event-driven stat keeps
	// for quantiles.
	sketchSize = 64
	// hourlySketchSize is how many samples an hourly stat keeps: a
	// week of hours.
	hourlySketchSize = 168
	// BaselineWindow is how far back samples count. Older ones are
	// dropped so the normal range follows the workload as it changes.
	BaselineWindow = 7 * 24 * time.Hour
	// maxWarningReasons bounds the Warning reasons learned per
	// workload; reasons beyond it are not learned.
	maxWarningReasons = 8
	// ewmaAlpha weighs a new sample in the moving average.
	ewmaAlpha = 0.2
	// madScale makes a MAD comparable to a standard deviation.
	madScale = 1.4826
	// madSpread is how many scaled MADs above the median still count
	// as normal.
	madSpread = 3.0
	// MinBaselineSamples is how many samples a stat needs before
	// IsUnusual judges anything.
	MinBaselineSamples = 10
	// unusualFactor is how far above the p95 a value must be.
	unusualFactor = 1.5
	// BaselineTTL is how long a workload's baseline is kept without a new
	// sample. A workload that has been gone this long is forgotten.
	BaselineTTL = 7 * 24 * time.Hour
	// maxRestoredGap is the longest gap since a restored baseline's last
	// sample that still counts as idle hours. A longer gap is most
	// likely kwatch downtime, whose hours were never watched.
	maxRestoredGap = time.Hour
)

// unusualFloor is the least absolute excess over the median that counts
// as unusual, so a very steady workload does not flag tiny wobbles.
var unusualFloor = map[Metric]float64{
	MetricRestartsPerHour: 2,
	MetricReadySeconds:    30,
	MetricPendingSeconds:  30,
	MetricJobSeconds:      60,
	MetricMemoryPeak:      64 << 20,
}

// floorOf is the unusual floor of metric; a Warning rate has its own.
func floorOf(metric Metric) float64 {
	if floor, ok := unusualFloor[metric]; ok {
		return floor
	}
	return 3
}

// Baseline is the persisted statistics of one workload.
type Baseline struct {
	Metrics map[Metric]Stat
	// HourStart is the start of the hour being counted. HourRestarts,
	// HourMemory and HourWarnings are what happened in it; each becomes
	// one sample when the hour ends.
	HourStart    time.Time          `json:",omitempty"`
	HourRestarts float64            `json:",omitempty"`
	HourMemory   float64            `json:",omitempty"`
	HourWarnings map[string]float64 `json:",omitempty"`
	// Template is the pod template (or revision) the memory samples
	// belong to. A new one starts the memory statistic again.
	Template string `json:",omitempty"`
	Updated  time.Time
	// marks are the restarts of the last hour; kwatch rebuilds them
	// after a restart, so they are not persisted.
	marks []restartMark
}

func (b Baseline) clone() Baseline {
	out := b
	out.Metrics = make(map[Metric]Stat, len(b.Metrics))
	for metric, stat := range b.Metrics {
		stat.Recent = append([]float64(nil), stat.Recent...)
		stat.At = append([]int64(nil), stat.At...)
		out.Metrics[metric] = stat
	}
	out.HourWarnings = make(map[string]float64, len(b.HourWarnings))
	for reason, count := range b.HourWarnings {
		out.HourWarnings[reason] = count
	}
	out.marks = append([]restartMark(nil), b.marks...)
	return out
}

// Baselines holds rolling statistics per workload, keyed by the
// workload's EntityID string. It is safe for concurrent use.
type Baselines struct {
	mu    sync.Mutex
	byKey map[string]*Baseline
	dirty map[string]bool
}

// NewBaselines builds an empty set.
func NewBaselines() *Baselines {
	return &Baselines{
		byKey: make(map[string]*Baseline), dirty: make(map[string]bool),
	}
}

// Len is how many workloads have a baseline.
func (b *Baselines) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.byKey)
}

// Add records one sample of metric for workload at at.
func (b *Baselines) Add(workload EntityID, metric Metric, value float64,
	at time.Time) {
	if workload.IsZero() || math.IsNaN(value) || value < 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	base := b.baselineLocked(workload.String())
	stat := base.Metrics[metric]
	stat.add(value, at, sketchSize)
	base.Metrics[metric] = stat
	base.Updated = at
}

func (b *Baselines) baselineLocked(key string) *Baseline {
	base := b.byKey[key]
	if base == nil {
		base = &Baseline{Metrics: make(map[Metric]Stat)}
		b.byKey[key] = base
	}
	b.dirty[key] = true
	return base
}

// Stat returns workload's statistic for metric.
func (b *Baselines) Stat(workload EntityID, metric Metric) (Stat, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	base := b.byKey[workload.String()]
	if base == nil {
		return Stat{}, false
	}
	stat, ok := base.Metrics[metric]
	stat.Recent = append([]float64(nil), stat.Recent...)
	stat.At = append([]int64(nil), stat.At...)
	return stat, ok && stat.Count > 0
}

// IsUnusual reports whether value is unusual for workload's metric.
//
// The rule: the statistic needs at least MinBaselineSamples samples, and
// value must exceed both 1.5 times the recent p95 and the recent median
// plus the metric's floor (2 restarts per hour, 30s ready or pending
// time, 60s Job duration). Without enough history nothing is unusual,
// so a new workload is never flagged by its baseline.
func (b *Baselines) IsUnusual(workload EntityID, metric Metric,
	value float64) bool {
	stat, ok := b.Stat(workload, metric)
	if !ok || stat.Count < MinBaselineSamples {
		return false
	}
	return value > unusualFactor*stat.Quantile(0.95) &&
		value > stat.Quantile(0.5)+unusualFloor[metric]
}

// TakeDirty returns detached copies of the baselines changed since the
// last call, keyed by workload, and marks them clean.
func (b *Baselines) TakeDirty() map[string]Baseline {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.dirty) == 0 {
		return nil
	}
	out := make(map[string]Baseline, len(b.dirty))
	for key := range b.dirty {
		out[key] = b.byKey[key].clone()
	}
	b.dirty = make(map[string]bool)
	return out
}

// MarkDirty marks the workloads in taken dirty again, after writing
// what TakeDirty returned failed. Newer samples are kept.
func (b *Baselines) MarkDirty(taken map[string]Baseline) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key := range taken {
		if b.byKey[key] != nil {
			b.dirty[key] = true
		}
	}
}

// Restore loads persisted baselines at now, when kwatch starts. Restored
// values are clean.
//
// The hours between the last sample and now were not all watched: kwatch
// was down for part of them. When that gap is longer than an hour, the
// restart hour window starts again at now instead of counting the
// unwatched hours as hours without restarts, which would flood the
// sketch with zeros.
func (b *Baselines) Restore(saved map[string]Baseline, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, base := range saved {
		copied := base.clone()
		if !copied.HourStart.IsZero() &&
			now.Sub(copied.Updated) > maxRestoredGap {
			copied.HourStart = now.Truncate(time.Hour)
			copied.HourRestarts, copied.HourMemory = 0, 0
			copied.HourWarnings = nil
		}
		b.byKey[key] = &copied
	}
}

// Expired removes the baselines of workloads that are gone and got no
// sample for BaselineTTL before now, so deleted workloads do not stay in
// memory forever. A workload that still exists keeps its baseline however
// quiet it is: a steady Deployment has no new pods for weeks. exists
// reports whether a workload is still in the model; nil treats every
// workload as gone. It returns the removed workload keys, sorted, so the
// caller can delete them from the store as well.
func (b *Baselines) Expired(
	now time.Time, exists func(EntityID) bool,
) []string {
	cutoff := now.Add(-BaselineTTL)
	b.mu.Lock()
	defer b.mu.Unlock()
	var removed []string
	for key, base := range b.byKey {
		if !base.Updated.Before(cutoff) || stillExists(key, exists) {
			continue
		}
		delete(b.byKey, key)
		delete(b.dirty, key)
		removed = append(removed, key)
	}
	sort.Strings(removed)
	return removed
}

func stillExists(key string, exists func(EntityID) bool) bool {
	if exists == nil {
		return false
	}
	id, ok := ParseEntityID(key)
	return ok && exists(id)
}
