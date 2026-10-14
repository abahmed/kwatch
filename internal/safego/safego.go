package safego

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// Run calls fn and turns a panic into an error, so a caller that already
// restarts or skips on an error (the component supervisor) backs off
// instead of crashing the process.
func Run(name string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			metrics.DefaultRegistry().IncPanic(name)
			err = fmt.Errorf("%s panicked: %v", name, r)
		}
	}()
	return fn()
}

// Go runs fn on a new goroutine; a panic is logged and counted and only
// that goroutine's work is lost. Use it for fan-out work that the next
// round or the next event repeats.
func Go(name string, fn func()) {
	go func() {
		if err := Run(name, func() error { fn(); return nil }); err != nil {
			klog.ErrorS(err, "goroutine panicked", "component", name)
		}
	}()
}

// Report logs the error Run returned for a panic; nil is ignored.
func Report(err error) {
	if err != nil {
		klog.ErrorS(err, "recovered panic")
	}
}

// Do calls fn and logs a panic instead of letting it escape.
func Do(name string, fn func()) {
	Report(Run(name, func() error { fn(); return nil }))
}
