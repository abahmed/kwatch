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
)

// Sketch sizes and smoothing.
const (
	// sketchSize is how many recent samples a stat keeps for quantiles.
	sketchSize = 64
	// ewmaAlpha weighs a new sample in the moving average.
	ewmaAlpha = 0.2
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
}

// Stat is one rolling statistic: an exponentially weighted mean and the
// most recent samples, which serve as a small quantile sketch.
type Stat struct {
	Count  int
	Mean   float64
	Recent []float64 `json:",omitempty"`
}

func (s *Stat) add(value float64) {
	if s.Count == 0 {
		s.Mean = value
	} else {
		s.Mean += ewmaAlpha * (value - s.Mean)
	}
	s.Count++
	s.Recent = append(s.Recent, value)
	if over := len(s.Recent) - sketchSize; over > 0 {
		s.Recent = append(s.Recent[:0:0], s.Recent[over:]...)
	}
}

// Quantile returns the q-quantile (0..1) of the recent samples by the
// nearest-rank method, or 0 without samples.
func (s Stat) Quantile(q float64) float64 {
	if len(s.Recent) == 0 {
		return 0
	}
	sorted := append([]float64(nil), s.Recent...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(q*float64(len(sorted)))) - 1
	return sorted[min(max(rank, 0), len(sorted)-1)]
}

// Baseline is the persisted statistics of one workload.
type Baseline struct {
	Metrics map[Metric]Stat
	// HourStart and HourRestarts count restarts in the current hour; the
	// count becomes a restarts_per_hour sample when the hour ends.
	HourStart    time.Time `json:",omitempty"`
	HourRestarts float64   `json:",omitempty"`
	Updated      time.Time
}

func (b Baseline) clone() Baseline {
	out := b
	out.Metrics = make(map[Metric]Stat, len(b.Metrics))
	for metric, stat := range b.Metrics {
		stat.Recent = append([]float64(nil), stat.Recent...)
		out.Metrics[metric] = stat
	}
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
	stat.add(value)
	base.Metrics[metric] = stat
	base.Updated = at
}

// AddRestarts counts restarts of workload's containers at at. Each full
// hour becomes one restarts_per_hour sample; hours without restarts
// count as zero, up to one sketch of them.
func (b *Baselines) AddRestarts(workload EntityID, restarts int,
	at time.Time) {
	if workload.IsZero() || restarts < 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	base := b.baselineLocked(workload.String())
	b.rollHourLocked(base, at)
	base.HourRestarts += float64(restarts)
	base.Updated = at
}

func (b *Baselines) rollHourLocked(base *Baseline, at time.Time) {
	hour := at.Truncate(time.Hour)
	if base.HourStart.IsZero() {
		base.HourStart = hour
		return
	}
	if !hour.After(base.HourStart) {
		return
	}
	stat := base.Metrics[MetricRestartsPerHour]
	stat.add(base.HourRestarts)
	idle := int(hour.Sub(base.HourStart)/time.Hour) - 1
	for i := 0; i < min(idle, sketchSize); i++ {
		stat.add(0)
	}
	base.Metrics[MetricRestartsPerHour] = stat
	base.HourStart, base.HourRestarts = hour, 0
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
			copied.HourRestarts = 0
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
