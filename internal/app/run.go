package app

import (
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// Run loads config and wires all monitors, then runs until shutdown.
func Run() int {
	return RunWithClock(clock.RealClock{}.Now)
}

// RunWithClock runs kwatch with an injected clock, primarily for deterministic
// integration tests and embedded callers.
//
// The edition is not fixed for the life of the process: the leader's
// license manager decides it. When a license starts or stops entitling Pro,
// the leader session ends and kwatch rebuilds in process, so every
// component is built with the new entitlements and the pod never restarts.
func RunWithClock(now func() time.Time) int {
	return newGenerationEnv(now).runAll(ResolveEntitlements())
}
