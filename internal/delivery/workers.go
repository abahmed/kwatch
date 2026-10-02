package delivery

import (
	"context"
	"errors"
	"time"
)

// workerProgressInterval is how often an idle worker reports progress.
// The application supervisor treats a component without progress as
// stalled; a quiet cluster sends nothing for long stretches, so an idle
// worker must still prove it is alive well within the stall deadline.
const workerProgressInterval = 10 * time.Second

// reconfigureDrainTimeout bounds how long the old generation may take to
// deliver its queued jobs before a reconfiguration gives up on it.
const reconfigureDrainTimeout = 10 * time.Second

// errGenerationStopping means workers from an earlier generation outlived
// their shutdown deadline and have not returned yet.
var errGenerationStopping = errors.New(
	"previous delivery generation is still stopping",
)

// workerSet is the bookkeeping for the provider workers one Start
// launched. The caller holds m.mu for every access.
type workerSet struct {
	// count is how many workers have not returned yet; done closes when
	// it reaches zero.
	count int
	done  chan struct{}
	// cancel ends the workers' lifecycle context.
	cancel context.CancelFunc
	// sendCtx carries provider requests. It outlives the workers' context
	// so a request in flight at SIGTERM can finish; Stop cancels it when
	// its own drain deadline ends.
	sendCtx    context.Context
	cancelSend context.CancelFunc
	// stuck is set when the workers outlived a shutdown deadline. Until
	// they return, no new generation may start.
	stuck bool
}

// launch prepares the set for count workers running under ctx and returns
// the context they run with. stuck is left as it is: it describes workers
// of an earlier set that may still be running.
func (w *workerSet) launch(ctx context.Context, count int) context.Context {
	var workerCtx context.Context
	workerCtx, w.cancel = context.WithCancel(ctx)
	w.sendCtx, w.cancelSend = context.WithCancel(context.WithoutCancel(ctx))
	w.done = make(chan struct{})
	w.count = count
	if count == 0 {
		close(w.done)
	}
	return workerCtx
}

// finishOne records that a worker returned. It reports whether that was
// the last one.
func (w *workerSet) finishOne() bool {
	if w.count == 0 {
		return false
	}
	w.count--
	if w.count != 0 || w.done == nil {
		return false
	}
	w.stuck = false
	close(w.done)
	return true
}

// stillStopping reports whether stuck workers have not returned yet.
func (w *workerSet) stillStopping() bool {
	return w.stuck && w.count > 0
}

// doneSignal is a channel that is closed at most once.
type doneSignal struct {
	ch     chan struct{}
	closed bool
}

func (d *doneSignal) ensure() {
	if d.ch == nil {
		d.ch = make(chan struct{})
	}
}

func (d *doneSignal) close() {
	d.ensure()
	if d.closed {
		return
	}
	d.closed = true
	close(d.ch)
}

// reconfigureOutcome is what WaitForReconfiguration reports. done is nil
// when no replacement result waits to be collected; otherwise it closes
// when the replacement finishes and err holds the result.
type reconfigureOutcome struct {
	done chan struct{}
	err  error
}
