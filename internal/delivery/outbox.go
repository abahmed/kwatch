package delivery

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/safego"
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
	// Attempts counts the sends that began and did not come back: a
	// crash or a provider panic. Zero for records of older versions.
	Attempts int `json:"attempts,omitempty"`
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
	// rewritten marks pending puts that replace a record which may
	// already be on disk, so removing the job must delete that record.
	rewritten map[string]struct{}
	// sweepEvery is the age sweep interval; tests shorten it.
	sweepEvery time.Duration
	// drainGrace is outboxDrainGrace unless a test shortens it.
	drainGrace time.Duration
	// after returns a channel that fires after d; tests replace it so a
	// failed write is retried without sleeping.
	after func(d time.Duration) <-chan time.Time
	// failedWrites counts consecutive failed batches, for the backoff.
	failedWrites int
	// sick reports that the writer keeps failing (true) or works again
	// (false); nil when nobody listens.
	sick func(bool)

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
		puts:      make(map[string]OutboxRecord),
		removes:   make(map[string]struct{}),
		live:      make(map[string]time.Time),
		rewritten: make(map[string]struct{}),
		wake:      make(chan struct{}, 1),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		after:     time.After,
	}
}

// outboxRetryBase and outboxRetryMax bound the wait before a failed batch
// is written again.
const (
	outboxRetryBase = time.Second
	outboxRetryMax  = 30 * time.Second
)

// retryDelay is the backoff after failedWrites consecutive failures.
func (o *outbox) retryDelay() time.Duration {
	delay := outboxRetryBase
	for i := 1; i < o.failedWrites && delay < outboxRetryMax; i++ {
		delay *= 2
	}
	return min(delay, outboxRetryMax)
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

func (o *outbox) removeLocked(id string) {
	delete(o.live, id)
	_, rewritten := o.rewritten[id]
	delete(o.rewritten, id)
	if _, pending := o.puts[id]; pending && !rewritten {
		// Never written, so there is nothing to delete on disk.
		delete(o.puts, id)
		return
	}
	delete(o.puts, id)
	o.removes[id] = struct{}{}
}

func (o *outbox) signal() {
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

// flush writes every buffered change in one batch and reports whether the
// write succeeded.
func (o *outbox) flush() bool {
	ok, _ := o.flushBatch()
	return ok
}

// flushBatch is flush that also reports whether there was a batch to
// write. A failed batch, or one whose write panicked, is put back before
// the panic goes on, so the next flush retries it.
func (o *outbox) flushBatch() (ok, wrote bool) {
	o.mu.Lock()
	if o.sealed {
		o.mu.Unlock()
		return true, false
	}
	puts, removes := o.puts, o.removes
	o.puts = make(map[string]OutboxRecord)
	o.removes = make(map[string]struct{})
	o.mu.Unlock()
	if len(puts) == 0 && len(removes) == 0 {
		return true, false
	}
	written := false
	defer func() {
		if !written {
			o.restoreBatch(puts, removes)
		}
	}()
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
		return false, true
	}
	written = true
	return true, true
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
		safego.Report(safego.Run("outbox-final", func() error {
			o.flush()
			return nil
		}))
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
