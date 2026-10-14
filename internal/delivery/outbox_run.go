package delivery

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/safego"
)

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
	safego.Go("outbox-writer", func() { o.supervise(ctx) })
}

// supervise keeps the writer running: a panic restarts it (see
// safego.Keep) instead of leaving alerts unpersisted, and done closes
// only when it is over for good. The restart loop ends when close is
// called or the drain grace after ctx has passed, so a backoff sleep
// never holds up close.
func (o *outbox) supervise(ctx context.Context) {
	defer close(o.done)
	keepCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer cancel()
		select {
		case <-o.stop:
		case <-keepCtx.Done():
		case <-ctx.Done():
			select {
			case <-o.stop:
			case <-keepCtx.Done():
			case <-time.After(o.graceAfterContext()):
			}
		}
	}()
	restarted := false
	safego.Keep(keepCtx, "outbox-writer", func(ready func()) error {
		defer func() { restarted = true }()
		return o.run(ctx, ready, restarted)
	}, o.sick)
}

// outboxDrainGrace bounds how long the writer keeps writing after its
// context ended. Delivery drains after that, and every job it delivers
// removes a record; a crash during the drain must not leave those
// records behind to be sent again. close ends the writer sooner.
const outboxDrainGrace = 30 * time.Second

// run writes batches and sweeps expired records until close is called, or
// until outboxDrainGrace after ctx ended.
// A restarted run starts with a flush, so a batch the failed run put back
// is retried at once. ready is called after each batch that was written.
func (o *outbox) run(
	ctx context.Context, ready func(), restarted bool,
) error {
	every := o.sweepEvery
	if every <= 0 {
		every = outboxSweepInterval
	}
	sweep := time.NewTicker(every)
	defer sweep.Stop()
	ended := ctx.Done()
	var grace, retry <-chan time.Time
	if restarted {
		retry = o.flushOrRetry(nil, ready)
	}
	for {
		select {
		case <-ended:
			ended = nil
			timer := time.NewTimer(o.graceAfterContext())
			defer timer.Stop()
			grace = timer.C
		case <-grace:
			return nil
		case <-o.stop:
			return nil
		case <-sweep.C:
			o.sweepExpired(o.now())
		case <-o.wake:
			retry = o.flushOrRetry(retry, ready)
		case <-retry:
			retry = o.flushOrRetry(nil, ready)
		}
	}
}

func (o *outbox) graceAfterContext() time.Duration {
	if o.drainGrace > 0 {
		return o.drainGrace
	}
	return outboxDrainGrace
}

// flushOrRetry writes the buffer and, when the batch failed, arms a
// backed-off retry unless one is already pending, so a failed write is not
// left waiting for the next change or the age sweep.
func (o *outbox) flushOrRetry(
	pending <-chan time.Time, ready func(),
) <-chan time.Time {
	ok, wrote := o.flushBatch()
	if ok {
		o.failedWrites = 0
		if wrote {
			ready()
		}
		return pending
	}
	o.failedWrites++
	if pending != nil {
		return pending
	}
	return o.after(o.retryDelay())
}
