// Package clock provides the real wall clock for constructors and legacy
// convenience paths. Time-sensitive decisions should receive an injected
// clock from the application instead of calling this package directly.
package clock

import "time"

// Clock is the small time dependency used by time-sensitive decisions.
type Clock interface {
	Now() time.Time
}

// Func adapts a function to Clock for existing composition code.
type Func func() time.Time

// Now implements Clock.
func (f Func) Now() time.Time { return f() }

// RealClock reads the process wall clock. Construct it at the composition
// root; domain packages should receive a Clock instead of creating one.
type RealClock struct{}

// Now implements Clock.
func (RealClock) Now() time.Time { return Now() }

// Now returns the real wall clock. It is intentionally immutable: tests and
// runtime components must inject their own clock through their constructors
// or setters rather than mutating process-wide state.
func Now() time.Time { return time.Now() }

// From returns the supplied clock or the real clock when no clock was
// provided. Constructors use it to keep time dependencies immutable after
// initialization while retaining convenient defaults for standalone callers.
func From(clocks []func() time.Time) func() time.Time {
	if len(clocks) > 0 && clocks[0] != nil {
		return clocks[0]
	}
	return Now
}
