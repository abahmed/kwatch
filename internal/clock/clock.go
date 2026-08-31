// Package clock provides the process-wide wall clock used by integrations
// that cannot receive a component-specific clock directly.
package clock

import (
	"sync/atomic"
	"time"
)

var current atomic.Value

func init() { current.Store((func() time.Time)(time.Now)) }

// Now returns the currently configured wall clock.
func Now() time.Time { return current.Load().(func() time.Time)() }

// Set replaces the process clock. It is safe for concurrent readers.
func Set(now func() time.Time) {
	if now != nil {
		current.Store(now)
	}
}
