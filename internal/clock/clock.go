// Package clock provides the time dependency used by time-sensitive code.
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
func (RealClock) Now() time.Time { return time.Now() }
