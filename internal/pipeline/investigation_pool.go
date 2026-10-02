package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/metrics"
)

// Investigation limits. Reads run on investigationWorkers goroutines; at
// most investigationQueue more wait. An announcement that finds no free
// slot is sent at once without output.
const (
	investigationWorkers = 3
	investigationQueue   = 16
	// investigationTimeout is the deadline of an investigation whose
	// plan names no budget of its own, and the most any budget may take.
	investigationTimeout = 5 * time.Second
	// outputWait is how long an announcement waits for its output on the
	// engine clock. After it the message goes without output.
	outputWait = 5 * time.Second
)

type investigationJob struct {
	id   string
	seq  int
	plan Investigation
}

type investigationResult struct {
	id string
	// seq tells a result apart from an older one of the same incident.
	seq    int
	kind   string
	result Result
}

// investigationPool runs investigations off the decision loop. The loop
// submits jobs and receives results; neither side ever blocks: results
// has room for every job that can be in flight.
type investigationPool struct {
	stats   *workerStats
	jobs    chan investigationJob
	results chan investigationResult
	// inFlight is owned by the loop: jobs submitted minus results taken.
	inFlight int
	workers  sync.WaitGroup
	done     chan struct{}
}

func newInvestigationPool(stats *workerStats) *investigationPool {
	return &investigationPool{
		stats: stats,
		jobs:  make(chan investigationJob, investigationQueue),
		results: make(chan investigationResult,
			investigationQueue+investigationWorkers),
		done: make(chan struct{}),
	}
}

// start runs the workers until ctx ends. done closes once all returned.
func (p *investigationPool) start(ctx context.Context) {
	for range investigationWorkers {
		p.workers.Add(1)
		go p.work(ctx)
	}
	go func() {
		p.workers.Wait()
		close(p.done)
	}()
}

func (p *investigationPool) work(ctx context.Context) {
	defer p.workers.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-p.jobs:
			p.results <- p.run(ctx, job)
		}
	}
}

// run investigates one job under its own budget.
func (p *investigationPool) run(
	ctx context.Context, job investigationJob,
) investigationResult {
	jobCtx, cancel := context.WithTimeout(ctx, budgetOf(job.plan))
	defer cancel()
	result := job.plan.Run(jobCtx)
	p.stats.investigated.Add(1)
	if jobCtx.Err() == context.DeadlineExceeded {
		p.stats.jobTimeouts.Add(1)
		metrics.DefaultRegistry().IncInvestigation("timeout")
	} else {
		metrics.DefaultRegistry().IncInvestigation("done")
	}
	return investigationResult{id: job.id, seq: job.seq,
		kind: job.plan.Kind, result: bounded(result)}
}

// budgetOf is the deadline of plan: its own budget, capped at
// investigationTimeout.
func budgetOf(plan Investigation) time.Duration {
	if plan.Budget <= 0 || plan.Budget > investigationTimeout {
		return investigationTimeout
	}
	return plan.Budget
}

// submit queues job. It reports false, without blocking, when no slot is
// free.
func (p *investigationPool) submit(job investigationJob) bool {
	if p.inFlight >= cap(p.results) {
		return false
	}
	select {
	case p.jobs <- job:
		p.inFlight++
		return true
	default:
		return false
	}
}

// received records that the loop took one result.
func (p *investigationPool) received() {
	p.inFlight--
}

// wait blocks until every worker returned or deadline fires, and reports
// whether they all returned.
func (p *investigationPool) wait(deadline <-chan time.Time) bool {
	select {
	case <-p.done:
		return true
	case <-deadline:
		return false
	}
}
