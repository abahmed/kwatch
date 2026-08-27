package correlation

import "github.com/abahmed/kwatch/internal/model"

// transition is a decided lifecycle change waiting to be announced.
type transition struct {
	inc    *model.Incident
	action model.IncidentAction
}

// emit hands finished transitions to the lifecycle hook.
//
// Every notification kwatch sends leaves the engine through this one function:
// a create or update decided by Process, a resolve, a group flush, a renotify,
// an escalation, a mass failure, a released symptom. Having a single exit is
// what keeps audit, diagnosis and delivery from drifting apart — when the
// handler notified on its own for the live path, alerts from that path were
// never audited and lost their diagnosis, and nobody noticed for a release.
//
// Skips are dropped here so callers can pass whatever they decided. The hook
// does I/O, so callers must not hold e.mu.
func (e *Engine) emit(ts ...transition) {
	hook := e.config.LifecycleHook
	if hook == nil {
		return
	}
	for _, t := range ts {
		if t.inc == nil || t.action == model.ActionSkip {
			continue
		}
		hook(t.inc, t.action)
	}
}

// publishBaseline snapshots the baseline under the lock and hands the copy to
// OnBaselineChange. Callers must not hold e.mu.
func (e *Engine) publishBaseline() {
	hook := e.config.OnBaselineChange
	if hook == nil {
		return
	}
	e.mu.Lock()
	snapshot := cloneBaseline(e.baseline)
	e.mu.Unlock()
	hook(snapshot)
}
