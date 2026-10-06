package replay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline"
)

// DefaultTail is how long a replay keeps ticking after the last entry, so
// settle and recovery holds that started near the end still finish.
const DefaultTail = 15 * time.Minute

// defaultMaxSteps bounds engine iterations, so an engine that keeps asking
// to be woken immediately fails the replay instead of spinning forever.
const defaultMaxSteps = 1_000_000

// Options tune a replay. The zero value is usable.
type Options struct {
	// Tail extends the replay past the last entry. Zero means DefaultTail.
	Tail time.Duration
	// MaxSteps bounds engine iterations. Zero means one million.
	MaxSteps int
	// SyncAt, when set, is the simulated time the replay tells the engine
	// that every source finished its initial list, as the application does
	// once its informers sync. Entries at or before SyncAt are submitted
	// first. A cold start then collects early announcements into one
	// startup summary. The zero value never signals the sync.
	SyncAt time.Time
	// Tick, when set with Every, is called each time simulated time
	// passes a multiple of Every after the log's start. The engine is
	// idle when it runs, so a soak test can measure memory or prune the
	// model at a steady point. It receives the simulated tick time.
	Tick  func(at time.Time)
	Every time.Duration
}

// CarriedDecision is a decision people heard through another message;
// see notification.Message.Carrier.
type CarriedDecision struct {
	Decision incident.Decision
	Carrier  string
	// At is the simulated time the decision was made. A roll-up carries
	// it at once; a digest or the startup summary later.
	At time.Time
}

// Result is everything the engine decided during a replay.
type Result struct {
	Decisions []incident.Decision
	Messages  []notification.Message
	// Times holds the simulated time each message was delivered.
	Times []time.Time
	// Carried are the decisions recorded for the audit log only, which
	// a digest, a roll-up or the startup summary carried.
	Carried []CarriedDecision
	// Incidents are the tracked incidents when the replay ended.
	Incidents []incident.Incident
	// End is the simulated time the replay stopped at.
	End time.Time
}

// Run replays log through a new engine built from deps. It replaces
// deps.Clock with a simulated clock and wraps deps.Sink so every decision
// is collected; deps must hold fresh state (a new Model and Manager) for
// the result to depend on the log alone.
//
// The simulated clock only moves when the engine is idle: it jumps to the
// earlier of the engine's next deadline and the next entry. Entries with
// the same timestamp are submitted in one call. No wall-clock time passes,
// so a replay of hours finishes in milliseconds and always decides the same.
func Run(
	ctx context.Context, log Log, deps pipeline.Dependencies, opts Options,
) (Result, error) {
	if log.Start.IsZero() {
		return Result{}, errors.New("replay: log without start")
	}
	sim := newSimClock(log.Start)
	var result Result
	next := deps.Sink
	deps.Clock = sim
	// Workers (storage batching, shutdown deadlines) wait on simulated
	// time too, so a replay never depends on how fast the host runs.
	deps.Timer = sim.Timer
	deps.Sink = func(
		ctx context.Context, d incident.Decision, m notification.Message,
	) {
		if m.Carrier != "" {
			result.Carried = append(result.Carried, CarriedDecision{
				Decision: d, Carrier: m.Carrier, At: sim.Now()})
			return
		}
		result.Decisions = append(result.Decisions, d)
		result.Messages = append(result.Messages, m)
		result.Times = append(result.Times, sim.Now())
		if next != nil {
			next(ctx, d, m)
		}
	}
	engine, err := pipeline.NewEngine(deps)
	if err != nil {
		return Result{}, err
	}
	d := driver{
		clock: sim, engine: engine, entries: log.Entries,
		end:      endOf(log, opts),
		maxSteps: opts.MaxSteps,
		syncAt:   opts.SyncAt, syncPending: !opts.SyncAt.IsZero(),
		tick: opts.Tick, every: opts.Every, nextTick: log.Start,
	}
	if d.every > 0 {
		d.nextTick = log.Start.Add(d.every)
	}
	if d.maxSteps <= 0 {
		d.maxSteps = defaultMaxSteps
	}
	if err := d.run(ctx); err != nil {
		return Result{}, err
	}
	result.Incidents = engine.Incidents()
	result.End = sim.Now()
	return result, nil
}

func endOf(log Log, opts Options) time.Time {
	tail := opts.Tail
	if tail <= 0 {
		tail = DefaultTail
	}
	last := log.Start
	if n := len(log.Entries); n > 0 {
		last = log.Entries[n-1].At
	}
	if opts.SyncAt.After(last) {
		last = opts.SyncAt
	}
	return last.Add(tail)
}

// driver steps the engine through a log on the simulated clock.
type driver struct {
	clock    *simClock
	engine   *pipeline.Engine
	entries  []Entry
	end      time.Time
	maxSteps int
	// syncAt is when SourcesSynced is signalled; syncPending is true
	// until it was.
	syncAt      time.Time
	syncPending bool
	// tick runs every `every`; nextTick is the next boundary.
	tick     func(time.Time)
	every    time.Duration
	nextTick time.Time
}

// ticks calls the tick hook for every boundary up to the time the clock
// is about to reach. The engine is idle, so the hook sees steady state.
func (d *driver) ticks(upTo time.Time) {
	if d.tick == nil || d.every <= 0 {
		return
	}
	for !d.nextTick.After(upTo) && !d.nextTick.After(d.end) {
		d.tick(d.nextTick)
		d.nextTick = d.nextTick.Add(d.every)
	}
}

func (d *driver) run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.engine.Run(ctx) }()
	stepErr := d.steps(ctx, done)
	// Finish before cancelling, so the shutdown deadlines the engine
	// creates as it stops are never fired early by the replay.
	d.clock.finish()
	cancel()
	close(d.clock.stopped)
	runErr := <-done
	if stepErr != nil {
		return stepErr
	}
	return runErr
}

// steps performs one action each time the engine is idle: fire its timer
// when it is due no later than the next event, otherwise submit the next
// batch of entries or signal the sync. It returns when the replay reaches
// its end.
func (d *driver) steps(ctx context.Context, done chan error) error {
	next := 0
	for step := 0; ; step++ {
		if step >= d.maxSteps {
			return fmt.Errorf("replay: engine exceeded %d steps", d.maxSteps)
		}
		var wait timerRequest
		select {
		case wait = <-d.clock.requests:
		case err := <-done:
			done <- err
			return errors.New("replay: engine stopped early")
		case <-ctx.Done():
			return ctx.Err()
		}
		due, kind := d.nextEvent(next)
		d.ticks(minTime(due, wait.at))
		switch {
		case !wait.at.After(due) && !wait.at.After(d.end):
			d.clock.set(wait.at)
			wait.fire <- wait.at
		case kind == eventEntries:
			d.clock.set(due)
			next = d.submit(ctx, next)
		case kind == eventSync:
			d.clock.set(due)
			d.syncPending = false
			d.engine.SourcesSynced()
		default:
			d.clock.set(d.end)
			return nil
		}
	}
}

// eventKind is what the driver does next when the engine timer is later.
type eventKind uint8

const (
	eventEnd eventKind = iota
	eventEntries
	eventSync
)

// nextEvent returns the next scheduled driver event after entries[:next].
// Entries at the sync time come first, as the initial list precedes the
// sync signal.
func (d *driver) nextEvent(next int) (time.Time, eventKind) {
	hasEntry := next < len(d.entries)
	if d.syncPending && !d.syncAt.After(d.end) &&
		(!hasEntry || d.syncAt.Before(d.entries[next].At)) {
		return d.syncAt, eventSync
	}
	if hasEntry {
		return d.entries[next].At, eventEntries
	}
	return d.end, eventEnd
}

// submit hands every entry sharing the timestamp of entries[from] to the
// engine in one call and returns the index of the first entry after them.
func (d *driver) submit(ctx context.Context, from int) int {
	at := d.entries[from].At
	var batch []inventory.Observation
	i := from
	for ; i < len(d.entries) && d.entries[i].At.Equal(at); i++ {
		batch = append(batch, d.entries[i].Observation)
	}
	d.engine.Submit(ctx, batch...)
	return i
}

// timerRequest is one engine wait: the deadline it asked for and the
// channel that wakes it.
type timerRequest struct {
	at   time.Time
	fire chan time.Time
}

// simClock is a pipeline.Clock whose time moves only when the driver sets
// it. After hands each wait to the driver, which is how the driver knows
// the engine has finished an iteration and is idle.
type simClock struct {
	mu       sync.Mutex
	now      time.Time
	requests chan timerRequest
	stopped  chan struct{}
	// workers are the pending worker timers, fired as time reaches them.
	workers []timerRequest
	// over is set once the replay stopped; see Timer.
	over bool
}

func newSimClock(start time.Time) *simClock {
	return &simClock{
		now: start, requests: make(chan timerRequest),
		stopped: make(chan struct{}),
	}
}

func (c *simClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *simClock) set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if at.After(c.now) {
		c.now = at
	}
	c.fireDueLocked()
}

// Timer is the worker timer of a replay. It fires when the simulated
// time reaches the deadline, so storage batches are cut at the same
// simulated moments on every run. Once the replay is over, simulated
// time no longer moves: the waits still pending fire at once, and later
// timers, which are only the shutdown deadlines of the workers, fall
// back to real time as a safety net that a healthy replay never reaches.
func (c *simClock) Timer(wait time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.over {
		return time.After(wait)
	}
	request := timerRequest{
		at: c.now.Add(max(wait, 0)), fire: make(chan time.Time, 1),
	}
	c.workers = append(c.workers, request)
	c.fireDueLocked()
	return request.fire
}

// finish marks the replay as over and fires every pending worker timer.
func (c *simClock) finish() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.over = true
	c.fireDueLocked()
}

func (c *simClock) fireDueLocked() {
	var kept []timerRequest
	for _, w := range c.workers {
		if c.over || !w.at.After(c.now) {
			w.fire <- w.at
			continue
		}
		kept = append(kept, w)
	}
	c.workers = kept
}

func (c *simClock) After(wait time.Duration) <-chan time.Time {
	request := timerRequest{
		at: c.Now().Add(max(wait, 0)), fire: make(chan time.Time, 1),
	}
	select {
	case c.requests <- request:
	case <-c.stopped:
	}
	return request.fire
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
