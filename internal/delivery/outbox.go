package delivery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

// The outbox persists every queued delivery so a crash or a shutdown that
// runs out of time does not lose it. A job is written when a provider
// queue accepts it and removed when that provider (or its fallback)
// accepted it, or when it was dead-lettered for good. The next session
// re-queues what is left, oldest first, before any new work.
//
// Writes are batched by one writer goroutine. Queueing a job only records
// it in memory, so the decision loop that calls NotifyIncident never waits
// for the disk.
const (
	// outboxMaxEntries bounds the persisted jobs; the oldest is dropped
	// first.
	outboxMaxEntries = 2048
	// outboxMaxAge drops jobs too old to be worth sending: at restore,
	// in a periodic sweep, and when a worker reaches such a job.
	outboxMaxAge = 24 * time.Hour
	// outboxFinalTimeout bounds the last write when delivery stops.
	outboxFinalTimeout = OutboxFinalTimeout
	// outboxSweepInterval is how often records past outboxMaxAge are
	// dropped while delivery runs, not only at the next restore.
	outboxSweepInterval = 10 * time.Minute
	// outboxRecordVersion is the persisted record layout. A record with
	// another version is dropped at restore, never misread.
	outboxRecordVersion = 1
)

// OutboxFinalTimeout bounds the last outbox write when delivery stops.
// The application's shutdown budget includes it.
const OutboxFinalTimeout = 2 * time.Second

// OutboxStore is the persistence port of the outbox. The application
// implements it over the state store.
type OutboxStore interface {
	// LoadOutbox returns every saved record in ID order.
	LoadOutbox() ([]OutboxRecord, error)
	// WriteOutbox stores puts, then removes the records named by removes.
	WriteOutbox(puts []OutboxRecord, removes []string) error
}

// OutboxRecord is one persisted delivery job for one provider.
type OutboxRecord struct {
	Version int `json:"version"`
	// ID orders records; it is a zero-padded sequence number.
	ID string `json:"id"`
	// Provider is the lookup name of the provider the job is queued for.
	Provider string `json:"provider"`
	// Message is the text of a plain message job.
	Message string `json:"message,omitempty"`
	// Incident is the message of an incident job.
	Incident *notification.Message `json:"incident,omitempty"`
	// Queued is when the job was first queued.
	Queued time.Time `json:"queued"`
}

// ErrOutboxBusy means the final outbox write did not return in time. The
// store must stay open, because the write may still be running.
var ErrOutboxBusy = errors.New("delivery outbox write still running")

// outbox buffers record changes for the writer goroutine.
type outbox struct {
	store OutboxStore
	now   func() time.Time

	mu      sync.Mutex
	seq     uint64
	puts    map[string]OutboxRecord
	removes map[string]struct{}
	// live is every record on disk or waiting to be written, with the
	// time its job was queued, for the size and age bounds.
	live map[string]time.Time
	// sweepEvery is the age sweep interval; tests shorten it.
	sweepEvery time.Duration
	// drainGrace is outboxDrainGrace unless a test shortens it.
	drainGrace time.Duration

	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	running bool
	stopped sync.Once
	// sealed is set after the final write; the store may be closed then.
	sealed bool
}

func newOutbox(store OutboxStore, now func() time.Time) *outbox {
	return &outbox{
		store: store, now: now,
		puts:    make(map[string]OutboxRecord),
		removes: make(map[string]struct{}),
		live:    make(map[string]time.Time),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func outboxID(seq uint64) string {
	return fmt.Sprintf("%020d", seq)
}

// add records a job queued for provider and returns its record ID. A nil
// outbox records nothing and returns "".
func (o *outbox) add(job deliverJob, provider string) string {
	if o == nil {
		return ""
	}
	queued := job.queued
	if queued.IsZero() {
		queued = o.now()
	}
	o.mu.Lock()
	o.seq++
	id := outboxID(o.seq)
	o.puts[id] = OutboxRecord{
		Version: outboxRecordVersion, ID: id, Provider: provider,
		Message: job.msg, Incident: job.incident, Queued: queued,
	}
	o.live[id] = queued
	o.enforceBoundLocked()
	o.publishDepthLocked()
	o.mu.Unlock()
	o.signal()
	return id
}

// remove forgets a record. Removing "" or an unknown ID is a no-op.
func (o *outbox) remove(id string) {
	if o == nil || id == "" {
		return
	}
	o.mu.Lock()
	o.removeLocked(id)
	o.publishDepthLocked()
	o.mu.Unlock()
	o.signal()
}

// publishDepthLocked reports the live records. The caller holds o.mu.
func (o *outbox) publishDepthLocked() {
	metrics.DefaultRegistry().Delivery.OutboxDepth.Store(int64(len(o.live)))
}

// sweepExpired drops the records whose jobs are older than outboxMaxAge.
// The jobs themselves are given up by the worker when it reaches them.
func (o *outbox) sweepExpired(now time.Time) {
	o.mu.Lock()
	for id, queued := range o.live {
		if now.Sub(queued) > outboxMaxAge {
			o.removeLocked(id)
			recordOutboxDrop(id, "expired")
		}
	}
	o.publishDepthLocked()
	o.mu.Unlock()
	o.signal()
}

func (o *outbox) removeLocked(id string) {
	delete(o.live, id)
	if _, pending := o.puts[id]; pending {
		// Never written, so there is nothing to delete on disk.
		delete(o.puts, id)
		return
	}
	o.removes[id] = struct{}{}
}

// enforceBoundLocked drops the oldest records over outboxMaxEntries. The
// job itself stays queued in memory; only its crash protection is lost.
func (o *outbox) enforceBoundLocked() {
	for len(o.live) > outboxMaxEntries {
		oldest := ""
		for id := range o.live {
			if oldest == "" || id < oldest {
				oldest = id
			}
		}
		o.removeLocked(oldest)
		recordOutboxDrop(oldest, "outbox_full")
	}
}

func recordOutboxDrop(id, reason string) {
	metrics.DefaultRegistry().OutboxDropped.Add(1)
	klog.V(2).InfoS("outbox record dropped",
		"component", "delivery", "operation", "outbox",
		"id", id, "reason", reason)
}

func (o *outbox) signal() {
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

// start launches the writer once. Later calls do nothing.
func (o *outbox) start(ctx context.Context) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.running {
		return
	}
	o.running = true
	go o.run(ctx)
}

// outboxDrainGrace bounds how long the writer keeps writing after its
// context ended. Delivery drains after that, and every job it delivers
// removes a record; a crash during the drain must not leave those
// records behind to be sent again. close ends the writer sooner.
const outboxDrainGrace = 30 * time.Second

// run writes batches and sweeps expired records until close is called, or
// until outboxDrainGrace after ctx ended.
func (o *outbox) run(ctx context.Context) {
	defer close(o.done)
	every := o.sweepEvery
	if every <= 0 {
		every = outboxSweepInterval
	}
	sweep := time.NewTicker(every)
	defer sweep.Stop()
	ended := ctx.Done()
	var grace <-chan time.Time
	for {
		select {
		case <-ended:
			ended = nil
			timer := time.NewTimer(o.graceAfterContext())
			defer timer.Stop()
			grace = timer.C
		case <-grace:
			return
		case <-o.stop:
			return
		case <-sweep.C:
			o.sweepExpired(o.now())
		case <-o.wake:
			o.flush()
		}
	}
}

func (o *outbox) graceAfterContext() time.Duration {
	if o.drainGrace > 0 {
		return o.drainGrace
	}
	return outboxDrainGrace
}

// flush writes every buffered change in one batch. A failed batch is put
// back, so the next flush retries it.
func (o *outbox) flush() {
	o.mu.Lock()
	if o.sealed {
		o.mu.Unlock()
		return
	}
	puts, removes := o.puts, o.removes
	o.puts = make(map[string]OutboxRecord)
	o.removes = make(map[string]struct{})
	o.mu.Unlock()
	if len(puts) == 0 && len(removes) == 0 {
		return
	}
	putList := make([]OutboxRecord, 0, len(puts))
	for _, record := range puts {
		putList = append(putList, record)
	}
	removeList := make([]string, 0, len(removes))
	for id := range removes {
		removeList = append(removeList, id)
	}
	if err := o.store.WriteOutbox(putList, removeList); err != nil {
		metrics.DefaultRegistry().OutboxWriteFailures.Add(1)
		klog.ErrorS(err, "write delivery outbox",
			"component", "delivery", "operation", "outbox")
		o.restoreBatch(puts, removes)
	}
}

// restoreBatch puts a failed batch back unless newer changes replaced it.
func (o *outbox) restoreBatch(
	puts map[string]OutboxRecord, removes map[string]struct{},
) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for id, record := range puts {
		if _, live := o.live[id]; live {
			o.puts[id] = record
		} else {
			// Removed while the batch was failing: it may be on disk.
			o.removes[id] = struct{}{}
		}
	}
	for id := range removes {
		o.removes[id] = struct{}{}
	}
}

// close stops the writer and makes the final write within the bound. It
// returns ErrOutboxBusy when that write did not return in time.
func (o *outbox) close(timeout time.Duration) error {
	if o == nil {
		return nil
	}
	o.stopped.Do(func() { close(o.stop) })
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	o.mu.Lock()
	running := o.running
	o.mu.Unlock()
	if running {
		// The loop returns after at most the flush it is running.
		select {
		case <-o.done:
		case <-timer.C:
			return ErrOutboxBusy
		}
	}
	final := make(chan struct{})
	go func() {
		defer close(final)
		o.flush()
		o.mu.Lock()
		o.sealed = true
		o.mu.Unlock()
	}()
	select {
	case <-final:
		return nil
	case <-timer.C:
		return ErrOutboxBusy
	}
}
