package opsgenie

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
)

// A close that finds no alert is not dropped and does not hold the
// provider's queue: Opsgenie creates alerts asynchronously and may need
// longer than a few seconds, and an alert that stays open pages for a
// problem that is over. The close is kept for later and retried in the
// background with growing waits for closeRetryWindow, the grace the close
// had before it was bounded. After that it is given up, logged and counted
// in kwatch_delivery_resolves_lost_total.
const (
	closeRetryFirst  = updateRetryDelay
	closeRetryCap    = time.Minute
	closeRetryWindow = 5 * time.Minute
	// maxPendingCloses bounds the closes waiting at once.
	maxPendingCloses = 64
)

// closeRetries are the closes kept for later: alerts that Opsgenie did
// not know yet when their incident resolved.
type closeRetries struct {
	mu      sync.Mutex
	pending map[string]*pendingClose
	// running counts the retry goroutines, so a test can wait for them.
	running sync.WaitGroup
}

// pendingClose is one close waiting to be sent again.
type pendingClose struct {
	cancel context.CancelFunc
}

// keepClose starts retrying the close of alias in the background and
// returns at once. The retry ends with ctx (shutdown), with a newer alert
// of the same alias (cancelClose), or when the window is over. A close
// already waiting for alias is the same close and is kept as it is.
func (o *Opsgenie) keepClose(ctx context.Context, alias string) {
	retryCtx, cancel := context.WithCancel(ctx)
	entry := &pendingClose{cancel: cancel}
	switch o.closes.add(alias, entry) {
	case closeAdded:
	case closeDuplicate:
		cancel()
		return
	default:
		cancel()
		o.giveUpClose(alias, "too many closes waiting")
		return
	}
	o.closes.running.Add(1)
	go func() {
		defer o.closes.running.Done()
		defer o.closes.remove(alias, entry)
		defer cancel()
		o.retryClose(retryCtx, alias, entry)
	}()
}

type closeAdd int

const (
	closeAdded closeAdd = iota
	closeDuplicate
	closeFull
)

// add records a pending close, unless alias has one or the bound is
// reached.
func (c *closeRetries) add(alias string, entry *pendingClose) closeAdd {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		c.pending = make(map[string]*pendingClose)
	}
	if _, ok := c.pending[alias]; ok {
		return closeDuplicate
	}
	if len(c.pending) >= maxPendingCloses {
		return closeFull
	}
	c.pending[alias] = entry
	return closeAdded
}

// remove forgets entry, unless a newer close of alias took its place.
func (c *closeRetries) remove(alias string, entry *pendingClose) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending[alias] == entry {
		delete(c.pending, alias)
	}
}

// stillPending reports whether entry was not dropped on purpose.
func (c *closeRetries) stillPending(alias string, e *pendingClose) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending[alias] == e
}

// cancelClose drops the pending close of alias: a new alert of the same
// alias must not be closed by the close of the old one.
func (o *Opsgenie) cancelClose(alias string) {
	o.closes.mu.Lock()
	entry := o.closes.pending[alias]
	delete(o.closes.pending, alias)
	o.closes.mu.Unlock()
	if entry != nil {
		entry.cancel()
	}
}

// retryClose sends the close again after each wait until it goes through,
// is rejected, or the window is over.
func (o *Opsgenie) retryClose(
	ctx context.Context, alias string, entry *pendingClose,
) {
	var waited time.Duration
	delay := closeRetryFirst
	for waited < closeRetryWindow {
		step := min(delay, closeRetryWindow-waited)
		if err := o.wait(ctx, step); err != nil {
			o.endCancelledClose(alias, entry)
			return
		}
		waited += step
		delay = min(delay*2, closeRetryCap)
		err := o.send(ctx, "POST", o.actionURL(alias, "close"),
			[]byte(`{}`))
		switch {
		case err == nil:
			return
		case ctx.Err() != nil:
			o.endCancelledClose(alias, entry)
			return
		case transport.IsPermanent(err) && !transport.IsNotFound(err):
			o.giveUpClose(alias, "rejected: "+err.Error())
			return
		}
	}
	o.giveUpClose(alias, "alert not found")
}

// endCancelledClose settles a retry whose context ended. A newer alert of
// the same alias dropped the close on purpose; shutdown gives it up.
func (o *Opsgenie) endCancelledClose(alias string, entry *pendingClose) {
	if o.closes.stillPending(alias, entry) {
		o.giveUpClose(alias, "stopped before the alert was found")
	}
}

// giveUpClose counts and logs a close that will not be sent: its alert
// may stay open until someone closes it by hand.
func (o *Opsgenie) giveUpClose(alias, reason string) {
	metrics.DefaultRegistry().DeliveryResolvesLost.Add(1)
	klog.InfoS("opsgenie close given up; the alert may stay open",
		"component", "delivery", "provider", o.Name(),
		"alias", alias, "reason", reason)
}
