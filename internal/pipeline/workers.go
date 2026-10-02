package pipeline

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// workerStats counts what the background workers did. Every field is
// written by a worker or the loop and read from any goroutine.
type workerStats struct {
	batches       atomic.Int64
	writeFailures atomic.Int64
	investigated  atomic.Int64
	skipped       atomic.Int64
	late          atomic.Int64
	jobTimeouts   atomic.Int64
	timeouts      atomic.Int64
}

// Stats is a point-in-time view of the engine's background workers. The
// counters only grow; a batch count that stops growing while decisions
// are made means the storage writer is stuck.
type Stats struct {
	// StoreBatches counts storage writes, each covering every snapshot
	// offered since the previous one.
	StoreBatches int64
	// StoreWriteFailures counts batches with at least one failed write.
	// Failed parts are retried with the next batch.
	StoreWriteFailures int64
	// Investigations counts finished investigations.
	Investigations int64
	// InvestigationsSkipped counts announcements sent without output
	// because every investigation worker and queue slot was busy.
	InvestigationsSkipped int64
	// InvestigationsLate counts announcements sent without output because
	// the investigation did not finish within the output wait.
	InvestigationsLate int64
	// InvestigationTimeouts counts investigations that hit their deadline.
	InvestigationTimeouts int64
	// ShutdownTimeouts counts workers that did not stop within their
	// shutdown deadline.
	ShutdownTimeouts int64
}

// Stats reports the background workers' counters. It is safe to call from
// any goroutine.
func (e *Engine) Stats() Stats {
	s := &e.stats
	return Stats{
		StoreBatches:          s.batches.Load(),
		StoreWriteFailures:    s.writeFailures.Load(),
		Investigations:        s.investigated.Load(),
		InvestigationsSkipped: s.skipped.Load(),
		InvestigationsLate:    s.late.Load(),
		InvestigationTimeouts: s.jobTimeouts.Load(),
		ShutdownTimeouts:      s.timeouts.Load(),
	}
}

// wallTimer is the default worker timer: real time, independent of the
// decision loop's clock.
func wallTimer(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// shutdownBudget bounds the whole pipeline shutdown. The storage writer,
// the history writer and the investigation pool share it, so Run
// returns within the application's 10-second component shutdown budget
// even when every worker is stuck, instead of waiting for each in turn.
const shutdownBudget = 8 * time.Second

// startWorkers starts the investigation pool and the storage writer. The
// returned stop hands the writer the final snapshot, cancels the pool and
// waits for every worker, history included, within one shared deadline.
func (e *Engine) startWorkers(ctx context.Context) func() {
	poolCtx, cancelPool := context.WithCancel(ctx)
	if pool := e.announcer.pool; pool != nil {
		pool.start(poolCtx)
	}
	e.storage.startIncidentWriter()
	return func() {
		e.save()
		cancelPool()
		deadline, release := sharedDeadline(e.deps.Timer, shutdownBudget)
		defer release()
		e.storage.askStop()
		e.stopWriter(deadline)
		e.stopHistory(deadline)
		e.stopPool(deadline)
	}
}

// sharedDeadline returns a channel that is closed once d has passed, so
// any number of waits can share one deadline, and a release that ends
// its timer goroutine early. The goroutine lives at most d.
func sharedDeadline(
	timer func(time.Duration) <-chan time.Time, d time.Duration,
) (<-chan time.Time, func()) {
	expired := make(chan time.Time)
	done := make(chan struct{})
	fired := timer(d)
	go func() {
		select {
		case <-fired:
			close(expired)
		case <-done:
		}
	}()
	var once sync.Once
	return expired, func() { once.Do(func() { close(done) }) }
}

func (e *Engine) stopWriter(deadline <-chan time.Time) {
	writer := e.storage.incidents
	if writer == nil || writer.close(deadline) {
		return
	}
	e.shutdownTimedOut("storage writer")
}

func (e *Engine) stopHistory(deadline <-chan time.Time) {
	if e.storage.history.stop(deadline) {
		return
	}
	e.shutdownTimedOut("history writer")
}

// WriterStopped reports whether the storage writers (incidents and history)
// have returned, or there never were any. After Run returns it is false
// when a writer was abandoned at its shutdown deadline: it may still write
// to the store, so the caller must not close the store.
func (e *Engine) WriterStopped() bool {
	return e.storage.stopped()
}

func (e *Engine) stopPool(deadline <-chan time.Time) {
	pool := e.announcer.pool
	if pool == nil || pool.wait(deadline) {
		return
	}
	e.shutdownTimedOut("investigation pool")
}

func (e *Engine) shutdownTimedOut(worker string) {
	e.stats.timeouts.Add(1)
	metrics.DefaultRegistry().ShutdownTimeouts.Add(1)
	klog.ErrorS(nil, "pipeline: worker did not stop in time",
		"component", "pipeline", "operation", "shutdown", "worker", worker)
}
