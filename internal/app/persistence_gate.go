package app

import "sync/atomic"

// persistenceGate fences saver writes after a leader loses its Lease. Saver
// shutdown uses independent short-lived contexts, so cancellation alone is
// not enough to prevent a final write from starting.
type persistenceGate struct {
	allowed atomic.Bool
}

func newPersistenceGate() *persistenceGate {
	gate := &persistenceGate{}
	gate.allowed.Store(true)
	return gate
}

func (g *persistenceGate) enable() {
	if g != nil {
		g.allowed.Store(true)
	}
}

func (g *persistenceGate) disable() {
	if g != nil {
		g.allowed.Store(false)
	}
}

func (g *persistenceGate) enabled() bool {
	return g == nil || g.allowed.Load()
}

func writesAllowed(canWrite func() bool) bool {
	return canWrite == nil || canWrite()
}
