package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/notice"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
	"github.com/abahmed/kwatch/internal/story"
)

// maxPending bounds facts waiting for the pipeline. Informers deliver in
// bursts during relists; beyond this, Submit blocks the caller briefly
// rather than dropping state.
const maxPending = 50000

// Sink receives every decision with its story. It is called from the
// pipeline goroutine and must not block for long.
type Sink func(ctx context.Context, d problem.Decision, m notice.Message)

// Clock supplies time and timers so tests can control both.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// heartbeat is the longest the loop sleeps. Each iteration reports
// progress, so a stuck pipeline is visible as a stall rather than hidden
// behind a separate heartbeat goroutine.
const heartbeat = 10 * time.Second

// Dependencies are the collaborators of the engine.
type Dependencies struct {
	Model     *knowledge.Model
	Detectors *signal.Registry
	Problems  *problem.Manager
	Sink      Sink
	// Writer composes messages; the zero value works.
	Writer story.Writer
	Clock  Clock
	// Progress is called after every loop iteration. Optional.
	Progress func()
	// Store persists problems so a restart never repeats a message.
	// Optional; without it problems live in memory only.
	Store ProblemStore
	// Investigate gathers application output for an announcement.
	// Optional.
	Investigate func(context.Context, problem.Problem) []string
	// InScope reports whether people want to hear about a problem.
	// Decisions for problems out of scope are dropped before delivery; the
	// problem is still tracked so reasoning keeps its evidence. Optional.
	InScope func(problem.Problem) bool
}

// ProblemStore loads and saves problem records and object fingerprints.
type ProblemStore interface {
	LoadProblems() ([]problem.Record, error)
	SaveProblems([]problem.Record) error
	LoadFingerprints() (map[string]string, error)
	SaveFingerprints(map[string]any) error
}

// restoreGrace is how long restored problems wait for their signals to be
// re-raised before they may recover. It covers the longest detector
// threshold.
const restoreGrace = 10 * time.Minute

// maxInvestigationsPerTick bounds the log reads one pipeline iteration
// makes, so a storm of announcements cannot stall the pipeline.
const maxInvestigationsPerTick = 3

// saveInterval bounds how stale persisted problems can be when nothing is
// decided for a while.
const saveInterval = time.Minute

// Engine runs the pipeline.
type Engine struct {
	deps    Dependencies
	tracker *signal.Tracker

	mu      sync.Mutex
	pending []knowledge.Fact
	wake    chan struct{}
	space   chan struct{}
	synced  chan struct{}

	// saved holds fingerprints from the previous run until the initial
	// list has been compared with them; snapshots are not written before.
	saved      map[string]string
	reconciled bool
	// dirty carries entities touched outside step, evaluated next step.
	dirty []knowledge.EntityID
	// coldStart is true when no problems were restored. Announcements
	// until startupUntil are collected into one startup summary.
	coldStart    bool
	startupUntil time.Time
	startup      []problem.Decision
}

// startupWindow covers the initial list plus one settle period, so the
// problems that already existed are all in the summary.
const startupWindow = 2 * time.Minute

// NewEngine validates dependencies and builds an engine.
func NewEngine(deps Dependencies) (*Engine, error) {
	if deps.Model == nil || deps.Detectors == nil ||
		deps.Problems == nil || deps.Sink == nil || deps.Clock == nil {
		return nil, errors.New("core: incomplete dependencies")
	}
	return &Engine{
		deps: deps, tracker: signal.NewTracker(),
		wake:   make(chan struct{}, 1),
		space:  make(chan struct{}, 1),
		synced: make(chan struct{}, 1),
	}, nil
}

// Problems returns detached copies of the tracked problems. It is safe to
// call from any goroutine while the engine runs.
func (e *Engine) Problems() []problem.Problem {
	return e.deps.Problems.Problems()
}

// SourcesSynced tells the engine that every source finished its initial
// list, so changes made while kwatch was down can be detected.
func (e *Engine) SourcesSynced() {
	notify(e.synced)
}

// Submit queues facts for the pipeline. It blocks only while the queue is
// full, and returns when ctx ends.
func (e *Engine) Submit(ctx context.Context, facts ...knowledge.Fact) {
	for {
		e.mu.Lock()
		if len(e.pending)+len(facts) <= maxPending || len(e.pending) == 0 {
			e.pending = append(e.pending, facts...)
			e.mu.Unlock()
			notify(e.wake)
			return
		}
		e.mu.Unlock()
		select {
		case <-e.space:
		case <-ctx.Done():
			return
		}
	}
}

// Run processes facts, rechecks and lifecycle deadlines until ctx ends.
func (e *Engine) Run(ctx context.Context) error {
	if err := e.restore(); err != nil {
		return err
	}
	defer e.save()
	checks := newRechecks()
	var nextTick time.Time
	lastSave := e.deps.Clock.Now()
	for {
		timer := e.timer(checks, nextTick)
		select {
		case <-ctx.Done():
			return nil
		case <-e.wake:
		case <-timer:
		case <-e.synced:
			e.reconcileDowntime()
		}
		now := e.deps.Clock.Now()
		var decided bool
		nextTick, decided = e.step(ctx, now, checks)
		if decided || now.Sub(lastSave) >= saveInterval {
			e.save()
			lastSave = now
		}
		if e.deps.Progress != nil {
			e.deps.Progress()
		}
	}
}

func (e *Engine) restore() error {
	if e.deps.Store == nil {
		e.coldStart = true
		return nil
	}
	records, err := e.deps.Store.LoadProblems()
	if err != nil {
		return fmt.Errorf("core: restore problems: %w", err)
	}
	e.coldStart = len(records) == 0
	e.deps.Problems.Restore(records, e.deps.Clock.Now().Add(restoreGrace))
	saved, err := e.deps.Store.LoadFingerprints()
	if err != nil {
		return fmt.Errorf("core: restore fingerprints: %w", err)
	}
	e.saved = saved
	return nil
}

// reconcileDowntime applies pending facts, then records every tracked
// object that changed while kwatch was down as a change.
func (e *Engine) reconcileDowntime() {
	if e.reconciled {
		return
	}
	e.reconciled = true
	if e.coldStart {
		e.startupUntil = e.deps.Clock.Now().Add(startupWindow)
	}
	e.dirty = append(e.dirty, e.drain()...)
	facts := DowntimeChanges(e.deps.Model, e.saved, e.deps.Clock.Now())
	e.saved = nil
	if len(facts) > 0 {
		klog.InfoS("core: changes made while kwatch was down",
			"component", "core", "count", len(facts))
		e.Submit(context.Background(), facts...)
	}
}

// save persists problems. A failed save is logged, not fatal: the next
// save retries, and delivery must not stop because the disk is slow.
func (e *Engine) save() {
	if e.deps.Store == nil {
		return
	}
	if err := e.deps.Store.SaveProblems(e.deps.Problems.Export()); err != nil {
		klog.ErrorS(err, "core: save problems", "component", "core")
	}
	if !e.reconciled {
		return
	}
	if err := e.deps.Store.SaveFingerprints(
		Fingerprints(e.deps.Model, e.deps.Clock.Now())); err != nil {
		klog.ErrorS(err, "core: save fingerprints", "component", "core")
	}
}

// step runs one pipeline iteration at now. It returns the next lifecycle
// deadline and whether any decision was made.
func (e *Engine) step(
	ctx context.Context, now time.Time, checks *rechecks,
) (time.Time, bool) {
	dirty := e.dirty
	e.dirty = nil
	dirty = append(dirty, e.drain()...)
	dirty = append(dirty, checks.due(now)...)
	e.evaluate(ctx, now, dirty, checks)
	return e.tick(ctx, now)
}

func (e *Engine) timer(
	checks *rechecks, nextTick time.Time,
) <-chan time.Time {
	now := e.deps.Clock.Now()
	wait := heartbeat
	for _, at := range []time.Time{checks.next(), nextTick} {
		if !at.IsZero() {
			wait = min(wait, max(at.Sub(now), 0))
		}
	}
	return e.deps.Clock.After(wait)
}

// drain applies pending facts to the model and returns the touched
// entities, each once.
func (e *Engine) drain() []knowledge.EntityID {
	e.mu.Lock()
	facts := e.pending
	e.pending = nil
	e.mu.Unlock()
	notify(e.space)
	seen := make(map[knowledge.EntityID]bool)
	var dirty []knowledge.EntityID
	for _, fact := range facts {
		update, err := e.deps.Model.Apply(fact)
		if err != nil {
			klog.V(2).InfoS("core: fact rejected", "error", err,
				"entity", fact.Entity.String())
			continue
		}
		for _, id := range update.Touched {
			if !seen[id] {
				seen[id] = true
				dirty = append(dirty, id)
			}
		}
	}
	return dirty
}

func (e *Engine) evaluate(
	ctx context.Context, now time.Time,
	dirty []knowledge.EntityID, checks *rechecks,
) {
	query := reason.Query{
		Model: e.deps.Model, Signals: e.tracker, Now: now,
	}
	var transitions []signal.Transition
	for _, id := range dirty {
		result := e.deps.Detectors.Evaluate(e.deps.Model, now, id)
		if result.RecheckAfter > 0 {
			checks.schedule(id, now.Add(result.RecheckAfter))
		}
		transitions = append(transitions,
			e.tracker.Observe(id, result.Signals)...)
	}
	if len(transitions) > 0 && ctx.Err() == nil {
		e.deps.Problems.Apply(query, transitions)
	}
}

func (e *Engine) tick(
	ctx context.Context, now time.Time,
) (time.Time, bool) {
	decisions, next := e.deps.Problems.Tick(now)
	decisions = e.inScope(decisions)
	decisions = e.collectStartup(ctx, now, decisions)
	for i, d := range decisions {
		if d.Action == problem.Announce && e.deps.Investigate != nil &&
			i < maxInvestigationsPerTick {
			d.Output = e.deps.Investigate(ctx, d.Problem)
		}
		e.deps.Sink(ctx, d, e.deps.Writer.Write(d, now))
	}
	if next == 0 {
		return time.Time{}, len(decisions) > 0
	}
	return now.Add(next), len(decisions) > 0
}

func (e *Engine) inScope(decisions []problem.Decision) []problem.Decision {
	if e.deps.InScope == nil {
		return decisions
	}
	kept := decisions[:0]
	for _, d := range decisions {
		if e.deps.InScope(d.Problem) {
			kept = append(kept, d)
		}
	}
	return kept
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// collectStartup holds cold-start announcements until the startup window
// ends, then sends them as one summary. Other decisions pass through.
func (e *Engine) collectStartup(
	ctx context.Context, now time.Time, decisions []problem.Decision,
) []problem.Decision {
	if e.startupUntil.IsZero() {
		return decisions
	}
	var rest []problem.Decision
	for _, d := range decisions {
		if d.Action == problem.Announce {
			e.startup = append(e.startup, d)
		} else {
			rest = append(rest, d)
		}
	}
	if now.Before(e.startupUntil) {
		return rest
	}
	if len(e.startup) > 0 {
		e.deps.Sink(ctx, problem.Decision{Reason: "startup summary"},
			story.StartupSummary(e.startup, now))
	}
	e.startup, e.startupUntil = nil, time.Time{}
	return rest
}
