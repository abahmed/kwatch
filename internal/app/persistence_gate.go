package app

import (
	"context"
	"sync/atomic"
	"time"
)

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

// finalWriteContext creates the bounded context used after a component
// receives cancellation. The component context is intentionally detached so
// a pending snapshot can finish, while canWrite still fences leadership loss.
func finalWriteContext(parent context.Context) (
	context.Context,
	context.CancelFunc,
) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(
		context.WithoutCancel(parent), 5*time.Second,
	)
}
