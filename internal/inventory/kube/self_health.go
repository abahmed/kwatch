package kube

import (
	"runtime"
	"time"

	"k8s.io/klog/v2"
)

// selfHealthEvery is how often kwatch logs its own memory use. A slow
// climb in the heap or in a map's size shows up in these lines long
// before the container's memory limit does.
const selfHealthEvery = 30 * time.Minute

// selfHealthClock says when the next self-health line is due.
type selfHealthClock struct {
	last time.Time
}

// due reports whether a line is due at now, and counts it as logged.
// The first call is due, so a restart logs its starting point.
func (c *selfHealthClock) due(now time.Time) bool {
	if !c.last.IsZero() && now.Sub(c.last) < selfHealthEvery {
		return false
	}
	c.last = now
	return true
}

// runtimeFields are the Go runtime's own numbers, as key/value pairs for
// a log line: heap in use, heap held from the system, goroutines.
func runtimeFields() []any {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return []any{
		"heapAllocMiB", stats.HeapAlloc >> 20,
		"heapInuseMiB", stats.HeapInuse >> 20,
		"goroutines", runtime.NumGoroutine(),
	}
}

// size is how many entries the log holds.
func (l *reachLog) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.failures)
}

// size is how many containers the log holds.
func (l *memoryLog) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.containers)
}

// size is how many counters are remembered.
func (c *counterRates) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.previous)
}

// size is how many volumes have samples.
func (g *growthTracker) size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.samples)
}

// selfHealthFields are the stats poller's bounded maps and the runtime
// numbers, as key/value pairs for the self-health line.
func (p *StatsPoller) selfHealthFields() []any {
	return append(runtimeFields(),
		"memoryLogContainers", p.memory.size(),
		"reachLogNodes", p.reach.size(),
		"counterSamples", p.counters.size(),
		"growthVolumes", p.growth.size())
}

// logSelfHealth writes the self-health line when it is due.
func (p *StatsPoller) logSelfHealth(now time.Time) {
	if !p.health.due(now) {
		return
	}
	klog.InfoS("self-health", append([]any{
		"component", "self-health"}, p.selfHealthFields()...)...)
}

// selfHealthFields are the crash-log round's remembered runs and
// back-offs.
func (r *CrashLogRound) selfHealthFields() []any {
	return []any{"crashLogRuns", len(r.read),
		"crashLogBackoffs", len(r.retryAt)}
}

// logSelfHealth writes the crash-log sizes when the line is due.
func (r *CrashLogRound) logSelfHealth(now time.Time) {
	if !r.health.due(now) {
		return
	}
	klog.InfoS("self-health", append([]any{
		"component", "self-health-crash-log"}, r.selfHealthFields()...)...)
}
