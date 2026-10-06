package inventory

import (
	"math"
	"sort"
	"time"
)

// restartMark is a number of restarts seen at one moment.
type restartMark struct {
	At time.Time
	N  float64
}

// maxRestartMarks bounds the restarts remembered for the last hour.
const maxRestartMarks = 64

// Touch makes sure workload has a baseline and brings its hour counters
// up to at, so quiet hours are counted as quiet. It is cheap and is
// meant to be called whenever one of the workload's pods is seen.
func (b *Baselines) Touch(workload EntityID, at time.Time) {
	if workload.IsZero() {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := workload.String()
	base := b.byKey[key]
	if base == nil || base.HourStart.IsZero() {
		base = b.baselineLocked(key)
		base.HourStart, base.Updated = at.Truncate(time.Hour), at
		return
	}
	if at.Truncate(time.Hour).After(base.HourStart) {
		b.dirty[key] = true
		b.rollHourLocked(base, at)
		base.Updated = at
	}
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
	base.marks = append(base.marks, restartMark{at, float64(restarts)})
	if over := len(base.marks) - maxRestartMarks; over > 0 {
		base.marks = append(base.marks[:0:0], base.marks[over:]...)
	}
	base.Updated = at
}

// RestartsInLastHour is how many container restarts of workload kwatch
// saw in the hour before now. It counts only what this process watched.
func (b *Baselines) RestartsInLastHour(
	workload EntityID, now time.Time,
) float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	base := b.byKey[workload.String()]
	if base == nil {
		return 0
	}
	total := 0.0
	for _, mark := range base.marks {
		if now.Sub(mark.At) < time.Hour {
			total += mark.N
		}
	}
	return total
}

// AddMemory records a container memory reading of workload at at. The
// highest reading of each hour becomes one memory_peak_bytes sample.
func (b *Baselines) AddMemory(workload EntityID, bytes float64,
	at time.Time) {
	if workload.IsZero() || math.IsNaN(bytes) || bytes <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	base := b.baselineLocked(workload.String())
	b.rollHourLocked(base, at)
	base.HourMemory = math.Max(base.HourMemory, bytes)
	base.Updated = at
}

// AddWarning counts Warning events of one reason for workload at at.
// Only the first maxWarningReasons reasons of a workload are learned.
func (b *Baselines) AddWarning(workload EntityID, reason string,
	count int, at time.Time) {
	if workload.IsZero() || count <= 0 || reason == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	base := b.baselineLocked(workload.String())
	b.rollHourLocked(base, at)
	_, counted := base.HourWarnings[reason]
	_, learned := base.Metrics[WarningMetric(reason)]
	if !counted && !learned && base.warningReasons() >= maxWarningReasons {
		return
	}
	if base.HourWarnings == nil {
		base.HourWarnings = make(map[string]float64)
	}
	base.HourWarnings[reason] += float64(count)
	base.Updated = at
}

// warningReasons counts the Warning reasons the baseline learns.
func (base *Baseline) warningReasons() int {
	seen := make(map[Metric]bool)
	for metric := range base.Metrics {
		if isWarningMetric(metric) {
			seen[metric] = true
		}
	}
	for reason := range base.HourWarnings {
		seen[WarningMetric(reason)] = true
	}
	return len(seen)
}

func isWarningMetric(metric Metric) bool {
	return len(metric) > len(warningMetricPrefix) &&
		string(metric[:len(warningMetricPrefix)]) == warningMetricPrefix
}

// Rebase tells the baseline which pod template workload runs now. A new
// template starts the memory statistic again, because new code has new
// memory; restart, readiness and event statistics carry on, since a bad
// release is judged against how the workload behaved before it.
func (b *Baselines) Rebase(workload EntityID, template string,
	at time.Time) {
	if workload.IsZero() || template == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	base := b.byKey[workload.String()]
	if base != nil && base.Template == template {
		return
	}
	base = b.baselineLocked(workload.String())
	if base.Template != "" {
		delete(base.Metrics, MetricMemoryPeak)
		base.HourMemory = 0
	}
	base.Template, base.Updated = template, at
}

// rollHourLocked turns the finished hours of base into samples. The
// first finished hour carries the counters; hours after it were quiet.
func (b *Baselines) rollHourLocked(base *Baseline, at time.Time) {
	hour := at.Truncate(time.Hour)
	if base.HourStart.IsZero() {
		base.HourStart = hour
		return
	}
	if !hour.After(base.HourStart) {
		return
	}
	idle := int(hour.Sub(base.HourStart)/time.Hour) - 1
	base.rollSeries(MetricRestartsPerHour, base.HourRestarts, idle, true)
	if base.HourMemory > 0 {
		base.rollSeries(MetricMemoryPeak, base.HourMemory, 0, false)
	}
	for _, reason := range base.warningNames() {
		base.rollSeries(WarningMetric(reason),
			base.HourWarnings[reason], idle, true)
	}
	base.HourStart, base.HourRestarts, base.HourMemory = hour, 0, 0
	base.HourWarnings = nil
}

// warningNames lists every learned or counted Warning reason, sorted.
func (base *Baseline) warningNames() []string {
	seen := make(map[string]bool)
	for metric := range base.Metrics {
		if isWarningMetric(metric) {
			seen[string(metric[len(warningMetricPrefix):])] = true
		}
	}
	for reason := range base.HourWarnings {
		seen[reason] = true
	}
	names := make([]string, 0, len(seen))
	for reason := range seen {
		names = append(names, reason)
	}
	sort.Strings(names)
	return names
}

// rollSeries appends the finished hour's value and, for a rate, the
// quiet hours after it, to metric's statistic.
func (base *Baseline) rollSeries(
	metric Metric, value float64, idle int, rate bool,
) {
	stat := base.Metrics[metric]
	start := base.HourStart
	stat.add(value, start.Add(time.Hour), hourlySketchSize)
	if rate {
		for i := 0; i < min(idle, hourlySketchSize); i++ {
			at := start.Add(time.Duration(i+2) * time.Hour)
			stat.add(0, at, hourlySketchSize)
		}
	}
	base.Metrics[metric] = stat
}
