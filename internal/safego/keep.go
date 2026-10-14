package safego

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// The restart backoff of everything that must keep running: it starts at
// RestartInitial and doubles up to RestartMaximum.
const (
	RestartInitial = time.Second
	RestartMaximum = time.Minute
	// SickAfter is how many failures in a row make Keep report the
	// work as sick.
	SickAfter = 3
)

// Wait sleeps for d and reports false when ctx ended first. Tests
// replace it so a restart does not sleep.
var Wait = func(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Backoff is the wait before restart number n (counting from 1).
func Backoff(n int) time.Duration {
	delay := RestartInitial
	for i := 1; i < n && delay < RestartMaximum; i++ {
		delay *= 2
	}
	return min(delay, RestartMaximum)
}

// Keep runs fn and runs it again after a panic or an error, with the
// backoff above, until ctx ends or fn returns nil. It is for goroutines
// that nothing else restarts.
//
// fn calls ready after its first successful unit of work. That ends the
// failure streak, so a long-running fn is reported healthy as soon as it
// works, not only when it returns. After SickAfter failures in a row
// Keep calls sick(true); the next ready calls sick(false). sick may be
// nil.
func Keep(
	ctx context.Context, name string,
	fn func(ready func()) error, sick func(bool),
) {
	report := func(v bool) {
		if sick != nil {
			sick(v)
		}
	}
	var mu sync.Mutex
	failures, isSick := 0, false
	ready := func() {
		mu.Lock()
		defer mu.Unlock()
		failures = 0
		if isSick {
			isSick = false
			report(false)
		}
	}
	for ctx.Err() == nil {
		err := Run(name, func() error { return fn(ready) })
		if err == nil || ctx.Err() != nil {
			ready()
			return
		}
		mu.Lock()
		failures++
		n := failures
		becameSick := n >= SickAfter && !isSick
		isSick = isSick || becameSick
		mu.Unlock()
		klog.ErrorS(err, "restarting after failure",
			"component", name, "failures", n)
		if becameSick {
			report(true)
		}
		if !Wait(ctx, Backoff(n)) {
			return
		}
	}
}
