package app

import (
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
)

// decisionLogQueue bounds audit entries waiting for the writer. The
// decision loop never waits: an entry that finds the queue full is
// dropped and counted.
const decisionLogQueue = 4096

// decisionLogShutdown bounds how long the pipeline waits for the audit
// writer to drain after the engine stopped.
const decisionLogShutdown = 5 * time.Second

// decisionLog writes audit entries and refreshes the open-incident gauge
// off the decision loop. Both may be slow: the audit log is a file or
// stream, and the gauge copies every incident.
type decisionLog struct {
	write     func(audit.Entry)
	registry  *metrics.Registry
	incidents func() []incident.Incident

	entries chan audit.Entry
	// refresh asks the worker to recompute the gauge.
	refresh chan struct{}
	stop    chan struct{}
	stopped sync.Once
	done    chan struct{}
	dropped atomic.Int64
	written atomic.Int64
}

func newDecisionLog(
	write func(audit.Entry), reg *metrics.Registry,
	incidents func() []incident.Incident,
) *decisionLog {
	return &decisionLog{
		write: write, registry: reg, incidents: incidents,
		entries: make(chan audit.Entry, decisionLogQueue),
		refresh: make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// record queues entry and, for a counted decision, a gauge refresh. It
// never blocks.
func (l *decisionLog) record(entry audit.Entry, counted bool) {
	if counted {
		select {
		case l.refresh <- struct{}{}:
		default:
		}
	}
	select {
	case l.entries <- entry:
	default:
		l.registry.AuditDropped.Add(1)
		if l.dropped.Add(1) == 1 {
			klog.ErrorS(nil, "pipeline: audit queue full; dropping entries",
				"component", "pipeline", "operation", "audit")
		}
	}
}

// run writes queued entries until close, then drains what is left.
func (l *decisionLog) run() {
	defer close(l.done)
	for {
		select {
		case entry := <-l.entries:
			l.writeEntry(entry)
		case <-l.refresh:
			refreshOpenIncidents(l.registry, l.incidents)
		case <-l.stop:
			l.drain()
			return
		}
	}
}

func (l *decisionLog) drain() {
	for {
		select {
		case entry := <-l.entries:
			l.writeEntry(entry)
		default:
			refreshOpenIncidents(l.registry, l.incidents)
			return
		}
	}
}

func (l *decisionLog) writeEntry(entry audit.Entry) {
	l.write(entry)
	l.written.Add(1)
}

// closeWithin is close with a deadline of d that starts when closeWithin
// runs. Deferring close(time.After(d)) would be wrong: a deferred call
// evaluates its arguments at once, so the deadline would already have
// passed when a long-running pipeline stops.
func (l *decisionLog) closeWithin(
	d time.Duration, after func(time.Duration) <-chan time.Time,
) bool {
	return l.close(after(d))
}

// close stops the worker and waits for the drain at most until deadline
// fires. It reports whether the worker finished.
func (l *decisionLog) close(deadline <-chan time.Time) bool {
	l.stopped.Do(func() { close(l.stop) })
	select {
	case <-l.done:
		return true
	case <-deadline:
		l.registry.ShutdownTimeouts.Add(1)
		klog.ErrorS(nil, "pipeline: audit writer did not stop in time",
			"component", "pipeline", "operation", "shutdown")
		return false
	}
}
