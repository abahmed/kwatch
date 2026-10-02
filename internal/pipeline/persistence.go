package pipeline

import (
	"slices"
	"sync/atomic"
)

// persistence owns everything the engine keeps in its store. Two writers
// do the I/O, each on its own goroutine: the incident writer saves
// incident records, fingerprints and the startup marker, and the history
// writer saves the timeline and the baselines. The decision loop only
// hands them snapshots and never waits for a write.
//
// It also keeps the fingerprints read at start, until the initial list
// has been compared with them to find changes made while kwatch was down.
type persistence struct {
	// incidents writes the IncidentStore; nil without a Store.
	incidents *storeWriter
	// history feeds health marks, baselines and the timeline.
	history *historyRecorder
	// saved holds fingerprints from the previous run until reconciled is
	// set. Fingerprint snapshots are not written before, so a restart in
	// between still compares against the previous run.
	saved      map[string]string
	reconciled bool
	// started is set once the incident writer's goroutine runs. Run
	// returns before it when restore fails; a writer that never ran
	// counts as stopped, so the caller may close the store.
	started atomic.Bool
	// carried are the saved fingerprints of kinds that had not synced
	// when they were compared (see carriedKinds).
	carried carriedKinds
}

func newPersistence(deps Dependencies, stats *workerStats) *persistence {
	p := &persistence{history: newHistoryRecorder(deps, stats)}
	if deps.Store != nil {
		p.incidents = newStoreWriter(deps.Store, deps.Timer, stats)
	}
	return p
}

// startIncidentWriter starts the incident writer's goroutine.
func (p *persistence) startIncidentWriter() {
	if p.incidents != nil {
		p.started.Store(true)
		go p.incidents.run()
	}
}

// saveStartup hands the startup marker to the incident writer. A failed
// write is retried with the next batch; the worst case is one more
// startup summary.
func (p *persistence) saveStartup(state StartupState) {
	if p.incidents == nil {
		return
	}
	// The writer reads the marker on its own goroutine: hand it copies
	// the loop never appends to.
	state.Incidents = slices.Clone(state.Incidents)
	state.Followed = slices.Clone(state.Followed)
	state.Resolved = slices.Clone(state.Resolved)
	p.incidents.offer(storeSnapshot{startup: &state})
}

// askStop asks both writers for their final write without waiting, so
// the two writes run at the same time.
func (p *persistence) askStop() {
	if p.incidents != nil {
		p.incidents.askStop()
	}
	p.history.askStop()
}

// stopped reports whether both writers have returned, or never ran.
func (p *persistence) stopped() bool {
	if p.history != nil && !p.history.stopped() {
		return false
	}
	if p.incidents == nil || !p.started.Load() {
		return true
	}
	select {
	case <-p.incidents.done:
		return true
	default:
		return false
	}
}
