package pipeline

import (
	"context"
	"errors"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Sink receives every decision with its message. It is called from the
// decision loop, so it must hand the message off and return at once.
type Sink func(ctx context.Context, d incident.Decision, m notification.Message)

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
	Model     *inventory.Model
	Detectors *detection.Registry
	Incidents *incident.Manager
	Sink      Sink
	// Writer composes messages; the zero value works.
	Writer compose.Writer
	Clock  Clock
	// Progress is called after every loop iteration. Optional.
	Progress func()
	// Store persists incidents so a restart never repeats a message.
	// Optional; without it incidents live in memory only.
	Store IncidentStore
	// Investigator plans the investigation of each incident when it
	// opens. The reads run on a bounded worker pool under a per-kind
	// budget, never on the decision loop. Optional.
	Investigator Investigator
	// InScope reports whether people want to hear about an incident.
	// Decisions for incidents out of scope are dropped before delivery; the
	// incident is still tracked so reasoning keeps its evidence. Optional.
	InScope func(incident.Incident) bool
	// Synced reports whether a kind is fully watched, so rules conclude
	// that an object is missing only when its kind is known. Optional.
	Synced func(inventory.Kind) bool
	// Timer is the wall-clock timer of the background workers: storage
	// batching and shutdown deadlines. It is separate from Clock, which
	// only the decision loop waits on. Optional; defaults to time.After.
	Timer func(time.Duration) <-chan time.Time
}

// IncidentStore loads and saves incident records, object fingerprints and
// the startup summary marker.
type IncidentStore interface {
	LoadIncidents() ([]incident.Record, error)
	SaveIncidents([]incident.Record) error
	LoadFingerprints() (map[string]string, error)
	SaveFingerprints(map[string]any) error
	// LoadStartup reports false when no marker was ever saved.
	LoadStartup() (StartupState, bool, error)
	SaveStartup(StartupState) error
}

// restoreGrace is how long restored incidents wait for their findings to be
// re-raised before they may recover. It covers the longest detector
// threshold.
const restoreGrace = 10 * time.Minute

// saveInterval bounds how stale persisted incidents can be when nothing is
// decided for a while.
const saveInterval = time.Minute

// Engine runs the pipeline. One goroutine, Run, makes every decision:
// it drains submitted observations into the model, evaluates the touched
// entities, solves root cause, ticks the incident lifecycles and hands
// the decisions to the announcer. Slow work never runs there: the
// announcer's investigation pool and the persistence writers do it on
// goroutines of their own.
type Engine struct {
	deps    Dependencies
	tracker *detection.Tracker
	// inbox queues the observations sources submit.
	inbox *inbox
	// synced signals that every source finished its initial list.
	synced chan struct{}
	// dirty carries entities touched outside step, evaluated next step.
	dirty []inventory.EntityID
	// moved collects the entities whose relations, notes or changes
	// moved since the last solve; root causes upstream of them are
	// explained again.
	moved movedSet
	// findings are the active findings by entity, as the tracker
	// holds them. Every solve reads them.
	findings map[inventory.EntityID][]detection.Finding
	// announcer turns decisions into messages for the sink.
	announcer *announcer
	// storage writes incidents and history to the store.
	storage *persistence
	stats   workerStats
}

// NewEngine validates dependencies and builds an engine.
func NewEngine(deps Dependencies) (*Engine, error) {
	if deps.Model == nil || deps.Detectors == nil ||
		deps.Incidents == nil || deps.Sink == nil || deps.Clock == nil {
		return nil, errors.New("pipeline: incomplete dependencies")
	}
	if deps.Timer == nil {
		deps.Timer = wallTimer
	}
	e := &Engine{
		deps: deps, tracker: detection.NewTracker(),
		inbox:    newInbox(),
		synced:   make(chan struct{}, 1),
		findings: map[inventory.EntityID][]detection.Finding{},
	}
	e.storage = newPersistence(deps, &e.stats)
	e.announcer = newAnnouncer(deps, e.storage, &e.stats)
	e.announcer.advisories = e.activeAdvisories
	return e, nil
}

// Incidents returns detached copies of the tracked incidents. It is safe to
// call from any goroutine while the engine runs.
func (e *Engine) Incidents() []incident.Incident {
	return e.deps.Incidents.Incidents()
}

// SourcesSynced tells the engine that every source finished its initial
// list, so changes made while kwatch was down can be detected.
func (e *Engine) SourcesSynced() {
	notify(e.synced)
}

// Run processes observations, rechecks and lifecycle deadlines until ctx
// ends. The loop itself does no I/O: investigations and storage writes run
// on workers Run owns, and Run waits for them, within bounded deadlines,
// before it returns. Each iteration waits for something to do, then
// steps: drain, evaluate, tick and announce.
func (e *Engine) Run(ctx context.Context) error {
	if err := e.restore(); err != nil {
		return err
	}
	e.storage.history.start()
	stopWorkers := e.startWorkers(ctx)
	defer stopWorkers()
	checks := newRechecks()
	var nextTick time.Time
	gate := &saveGate{lastSave: e.deps.Clock.Now()}
	for {
		timer := e.timer(checks, nextTick)
		select {
		case <-ctx.Done():
			return nil
		case <-e.inbox.wake:
		case <-timer:
		case <-gate.armed:
			gate.armed = nil
		case <-e.synced:
			e.reconcileDowntime()
		case result := <-e.announcer.outputs():
			e.announcer.attachOutput(ctx, result)
		}
		now := e.deps.Clock.Now()
		var decided bool
		nextTick, decided = e.step(ctx, now, checks)
		e.persist(gate, now, decided)
		e.progress()
	}
}

// reconcileDowntime applies pending observations, then records every tracked
// object that changed while kwatch was down as a change. It runs on the
// pipeline goroutine, so the changes are applied directly: queueing them
// through Submit could wait on a full queue only this goroutine drains.
func (e *Engine) reconcileDowntime() {
	if e.storage.reconciled {
		return
	}
	e.storage.reconciled = true
	e.announcer.startup.openWindow(e.deps.Clock.Now())
	// Split before draining: a kind synced now has submitted its list.
	compare, carried := splitSaved(e.storage.saved, e.deps.Synced)
	e.storage.saved, e.storage.carried = nil, carried
	e.dirty = append(e.dirty, e.drain()...)
	e.dirty = append(e.dirty, e.applyDowntime(compare)...)
}

// applyDowntime applies the changes between saved fingerprints and the
// model and returns the touched entities.
func (e *Engine) applyDowntime(
	saved map[string]string,
) []inventory.EntityID {
	if saved == nil {
		return nil
	}
	observations := DowntimeChanges(
		e.deps.Model, saved, e.deps.Synced, e.deps.Clock.Now())
	if len(observations) == 0 {
		return nil
	}
	klog.InfoS("pipeline: changes made while kwatch was down",
		"component", "pipeline", "count", len(observations))
	return e.apply(observations)
}

// step runs one pipeline iteration at now. It returns the next lifecycle
// deadline and whether any decision was made.
func (e *Engine) step(
	ctx context.Context, now time.Time, checks *rechecks,
) (time.Time, bool) {
	dirty := e.dirty
	e.dirty = nil
	late := e.storage.carried.due(e.deps.Synced)
	dirty = append(dirty, e.drain()...)
	dirty = append(dirty, e.applyDowntime(late)...)
	dirty = append(dirty, checks.due(now)...)
	e.evaluate(ctx, now, dirty, checks)
	next, decided := e.tick(ctx, now)
	e.observeLag()
	return next, decided
}

func (e *Engine) timer(
	checks *rechecks, nextTick time.Time,
) <-chan time.Time {
	if len(e.dirty) > 0 {
		// Entities queued for the next step, such as the webhooks of a
		// Service that just lost its endpoints, are evaluated at once.
		return e.deps.Clock.After(0)
	}
	now := e.deps.Clock.Now()
	wait := heartbeat
	deadlines := []time.Time{checks.next(), nextTick, e.announcer.nextHeld(),
		e.announcer.nextDigest()}
	for _, at := range deadlines {
		if !at.IsZero() {
			wait = min(wait, max(at.Sub(now), 0))
		}
	}
	return e.deps.Clock.After(wait)
}

// apply folds observations into the model and returns the touched entities,
// each once.
func (e *Engine) apply(
	observations []inventory.Observation,
) []inventory.EntityID {
	seen := make(map[inventory.EntityID]bool)
	var dirty []inventory.EntityID
	for _, observation := range observations {
		moved := e.moves(observation)
		update, err := e.deps.Model.Apply(observation)
		if err != nil {
			klog.V(2).InfoS("pipeline: observation rejected", "error", err,
				"entity", observation.Entity.String())
			continue
		}
		e.storage.history.observed(observation)
		if moved {
			e.moved.add(observation.Entity)
		}
		touched := update.Touched
		touched = append(touched, e.readers(observation, update)...)
		for _, id := range touched {
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
	dirty []inventory.EntityID, checks *rechecks,
) {
	var transitions []detection.Transition
	for _, id := range dirty {
		result := e.deps.Detectors.Evaluate(e.deps.Model, now, id)
		if result.RecheckAfter > 0 {
			checks.schedule(id, now.Add(result.RecheckAfter))
		}
		changed := e.tracker.Observe(id, result.Findings)
		logTransitions(changed)
		transitions = append(transitions, changed...)
		e.keepFindings(id)
	}
	e.storage.history.transitions(now, transitions, e.tracker.Active)
	e.markDependents(transitions)
	if (len(transitions) == 0 && e.moved.len() == 0) || ctx.Err() != nil {
		return
	}
	// One solve per iteration: a storm of transitions is explained once.
	snapshot := explain.NewSnapshot(e.deps.Model, e.activeFindings(),
		e.deps.Synced, now)
	e.deps.Incidents.Apply(snapshot, e.moved.take(), transitions)
	e.announcer.investigateOpened(now)
}

// tick advances the incident lifecycles to now and hands their
// decisions to the announcer. It returns the next lifecycle deadline and
// whether anything was decided or sent.
func (e *Engine) tick(
	ctx context.Context, now time.Time,
) (time.Time, bool) {
	decisions, next := e.deps.Incidents.Tick(now)
	e.storage.history.decided(now, decisions)
	announced := e.announcer.announce(ctx, now, decisions)
	decided := len(decisions) > 0 || announced
	if next == 0 {
		return time.Time{}, decided
	}
	return now.Add(next), decided
}

func (e *Engine) progress() {
	if e.deps.Progress != nil {
		e.deps.Progress()
	}
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// logTransitions writes one debug line per finding change. It is off by
// default; -v=4 shows why an incident gains or keeps a member.
func logTransitions(transitions []detection.Transition) {
	if !klog.V(4).Enabled() {
		return
	}
	for _, t := range transitions {
		klog.V(4).InfoS("pipeline: finding "+transitionName(t.Kind),
			"component", "pipeline", "entity", t.Finding.Entity.String(),
			"reason", t.Finding.Reason)
	}
}

func transitionName(kind detection.TransitionKind) string {
	switch kind {
	case detection.Raised:
		return "raised"
	case detection.Cleared:
		return "cleared"
	}
	return "changed"
}
