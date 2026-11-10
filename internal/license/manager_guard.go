package license

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/safego"
)

// A panic in the licensing code (a server response or a cached lease that
// parses in a way nobody expected) must never take monitoring down. Every
// entry into the manager's own work is guarded: the panic is logged and
// counted, the manager keeps the state it last computed from its cached
// lease, and the loop tries again later.

// panicRetryDelay is the wait after a panic. It is the longest retry
// backoff: a panic that repeats logs once an hour, not in a tight loop.
var panicRetryDelay = retryBackoff[len(retryBackoff)-1]

// guard runs fn and reports whether it finished without a panic.
func (m *Manager) guard(step string, fn func()) bool {
	err := safego.Run("license", func() error { fn(); return nil })
	if err == nil {
		return true
	}
	klog.ErrorS(err, "license step failed; keeping the cached "+
		"license state", "component", "license", "step", step)
	return false
}

// guardedDelay is the delay fn returns, or panicRetryDelay when it
// panicked.
func (m *Manager) guardedDelay(fn func() time.Duration) time.Duration {
	delay := panicRetryDelay
	m.guard("sync", func() { delay = fn() })
	return delay
}

// run starts the renewal loop on its own goroutine, which closes done
// when ctx ends. first gives the wait before the first cycle. A panic
// restarts the loop after panicRetryDelay.
func (m *Manager) run(ctx context.Context, first func() time.Duration) {
	go func() {
		defer close(m.done)
		delay := first()
		for ctx.Err() == nil {
			if m.guard("renew", func() { m.loopFrom(ctx, delay) }) {
				return
			}
			delay = panicRetryDelay
		}
	}()
}
