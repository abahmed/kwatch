package opsgenie

import "sync"

// A reconfiguration replaces the provider without stopping the old one
// (its closes belong to alerts that are already over, see
// delivery/provider_stop.go). Its close of alias X would still be
// retried after the new instance opened a new alert X for the same
// problem coming back, and close that live alert. So a new alert
// cancels the pending close of its alias in every instance, the old
// generations' too.

// waitingCloses are the close retries of the instances that kept a
// close, old generations included. An instance with none waiting is
// forgotten on the next cancel.
var waitingCloses struct {
	mu  sync.Mutex
	all map[*closeRetries]struct{}
}

// noteWaiting remembers that c keeps a close.
func noteWaiting(c *closeRetries) {
	waitingCloses.mu.Lock()
	defer waitingCloses.mu.Unlock()
	if waitingCloses.all == nil {
		waitingCloses.all = map[*closeRetries]struct{}{}
	}
	waitingCloses.all[c] = struct{}{}
}

// cancelEverywhere drops the pending close of alias in every instance.
func cancelEverywhere(alias string) {
	waitingCloses.mu.Lock()
	all := make([]*closeRetries, 0, len(waitingCloses.all))
	for c := range waitingCloses.all {
		all = append(all, c)
	}
	waitingCloses.mu.Unlock()
	for _, c := range all {
		if c.cancel(alias) == 0 {
			forgetIdle(c)
		}
	}
}

// forgetIdle forgets c once no close waits in it.
func forgetIdle(c *closeRetries) {
	waitingCloses.mu.Lock()
	defer waitingCloses.mu.Unlock()
	c.mu.Lock()
	idle := len(c.pending) == 0
	c.mu.Unlock()
	if idle {
		delete(waitingCloses.all, c)
	}
}

// cancel drops the pending close of alias and reports how many closes
// still wait.
func (c *closeRetries) cancel(alias string) int {
	c.mu.Lock()
	entry := c.pending[alias]
	delete(c.pending, alias)
	left := len(c.pending)
	c.mu.Unlock()
	if entry != nil {
		entry.cancel()
	}
	return left
}
