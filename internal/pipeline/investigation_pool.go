package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/pipeline/investigate"
)

// Investigation limits. Reads run on investigationWorkers goroutines; at
// most investigationQueue more wait. An announcement that finds no free
// slot is sent at once without output.
const (
	investigationWorkers = 3
	investigationQueue   = 16
	// outputWait is how long an announcement waits for its output on the
	// engine clock. After it the message goes without output.
	outputWait = 5 * time.Second
	// abandonGrace is how long a worker waits, after a job's budget
	// ended, for an investigator to return. After it the worker takes
	// the next job and the stuck one is left to finish alone.
	abandonGrace = time.Second
	// maxAbandonedPerKind bounds the goroutines a kind of investigator
	// may leave running after its budget and grace. At the limit the kind
	// gets no new jobs until one of them returns: a stuck read leaks one
	// goroutine, never an unbounded number.
	maxAbandonedPerKind = 4
)

type investigationJob struct {
	id   string
	seq  int
	plan investigate.Investigation
}

type investigationResult struct {
	id string
	// seq tells a result apart from an older one of the same incident.
	seq    int
	kind   string
	result investigate.Result
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
	// grace is abandonGrace; tests shorten it.
	grace time.Duration
	// abandoned counts, per investigator kind, the goroutines left
	// running after their worker stopped waiting. Workers and the loop
	// both use it, so mu guards it.
	mu        sync.Mutex
	abandoned map[string]int
}

func newInvestigationPool(stats *workerStats) *investigationPool {
	return &investigationPool{
		stats: stats,
		jobs:  make(chan investigationJob, investigationQueue),
		results: make(chan investigationResult,
			investigationQueue+investigationWorkers),
		done:      make(chan struct{}),
		grace:     abandonGrace,
		abandoned: make(map[string]int),
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

// run investigates one job under its own budget. The budget holds even
// for an investigator that ignores its context: the job runs on its own
// goroutine and the worker stops waiting for it when the budget ends, so
// a stuck read costs one goroutine, not a worker. A panic in an
// investigator is logged and its result dropped (an empty one is
// returned, so the loop still counts the job as finished): reading logs
// and the model must never take the process down.
func (p *investigationPool) run(
	ctx context.Context, job investigationJob,
) investigationResult {
	jobCtx, cancel := context.WithTimeout(ctx, budgetOf(job.plan))
	defer cancel()
	finished := make(chan investigate.Result, 1)
	left := &leftBehind{}
	go func() {
		defer p.returned(job.plan.Kind, left)
		defer func() {
			if r := recover(); r != nil {
				klog.ErrorS(nil, "pipeline: investigation panicked",
					"component", "pipeline", "kind", job.plan.Kind,
					"panic", fmt.Sprint(r))
				finished <- investigate.Result{}
			}
		}()
		finished <- job.plan.Run(jobCtx)
	}()
	var result investigate.Result
	select {
	case result = <-finished:
	case <-jobCtx.Done():
		// An investigator stops at its context and hands back what it
		// found; one that does not within abandonGrace is left behind.
		select {
		case result = <-finished:
		case <-time.After(p.grace):
			p.abandon(job.plan.Kind, left)
		}
	}
	p.stats.investigated.Add(1)
	if jobCtx.Err() == context.DeadlineExceeded {
		p.stats.jobTimeouts.Add(1)
		metrics.DefaultRegistry().IncInvestigation("timeout")
	} else {
		metrics.DefaultRegistry().IncInvestigation("done")
	}
	return investigationResult{id: job.id, seq: job.seq,
		kind: job.plan.Kind, result: investigate.Bounded(result)}
}

// leftBehind tells the goroutine of a job whether its worker gave up on it.
// Both sides run on different goroutines; the pool's mutex guards it.
type leftBehind struct {
	abandoned, returned bool
}

// abandon counts the job's goroutine as left running, unless it already
// returned.
func (p *investigationPool) abandon(kind string, left *leftBehind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if left.returned {
		return
	}
	left.abandoned = true
	p.abandoned[kind]++
	p.stats.abandoned.Add(1)
	klog.InfoS("pipeline: investigation ignored its budget; "+
		"worker moved on", "component", "pipeline", "kind", kind,
		"abandoned", p.abandoned[kind])
}

// returned runs when the job's goroutine ends, however late.
func (p *investigationPool) returned(kind string, left *leftBehind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	left.returned = true
	if left.abandoned {
		p.abandoned[kind]--
	}
}

// atAbandonLimit reports a kind that left too many goroutines running.
func (p *investigationPool) atAbandonLimit(kind string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.abandoned[kind] >= maxAbandonedPerKind
}

// budgetOf is the deadline of plan: its own budget, capped at
// investigate.MaxBudget.
func budgetOf(plan investigate.Investigation) time.Duration {
	if plan.Budget <= 0 || plan.Budget > investigate.MaxBudget {
		return investigate.MaxBudget
	}
	return plan.Budget
}

// submit queues job. It reports false, without blocking, when no slot is
// free or the job's kind has too many abandoned goroutines.
func (p *investigationPool) submit(job investigationJob) bool {
	if p.inFlight >= cap(p.results) {
		return false
	}
	if p.atAbandonLimit(job.plan.Kind) {
		p.stats.refused.Add(1)
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
