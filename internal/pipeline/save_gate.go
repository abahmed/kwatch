package pipeline

import "time"

// saveGate decides when the loop builds a save snapshot. A decided
// iteration only marks the state dirty; the snapshot is built at most once
// per writeBatch, because the writer keeps just the latest one anyway.
type saveGate struct {
	dirty bool
	// lastSnapshot is when the last snapshot was built, for batching.
	lastSnapshot time.Time
	// lastSave is the last snapshot of any kind, for the idle refresh.
	lastSave time.Time
	// armed fires when a dirty state must be snapshotted; nil when idle.
	armed <-chan time.Time
}

// persist builds a snapshot when one is due at now. It arms the injected
// worker timer when a dirty state has to wait for the next batch.
func (e *Engine) persist(g *saveGate, now time.Time, decided bool) {
	g.dirty = g.dirty || decided
	switch {
	case g.dirty && !now.Before(g.lastSnapshot.Add(writeBatch)):
		e.snapshotNow(g, now)
	case !g.dirty && now.Sub(g.lastSave) >= saveInterval:
		e.snapshotNow(g, now)
	case g.dirty && g.armed == nil:
		g.armed = e.deps.Timer(writeBatch)
	}
}

func (e *Engine) snapshotNow(g *saveGate, now time.Time) {
	e.save()
	g.dirty, g.armed = false, nil
	g.lastSnapshot, g.lastSave = now, now
}
