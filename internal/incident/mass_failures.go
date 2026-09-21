package incident

import "github.com/abahmed/kwatch/internal/model"

// MassFailureSet returns a copy of tracked synthetic mass-failure incidents.
func (e *Engine) MassFailureSet() map[model.IncidentKey]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[model.IncidentKey]*model.Incident, len(e.massFailures))
	for key, inc := range e.massFailures {
		out[key] = inc.Clone()
	}
	return out
}

func (e *Engine) HasMassFailure(key model.IncidentKey) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.hasMassFailureLocked(key)
}

// hasMassFailureLocked is used by processing paths already holding e.mu.
func (e *Engine) hasMassFailureLocked(key model.IncidentKey) bool {
	_, ok := e.massFailures[key]
	return ok
}

// AddMassFailure registers and announces a synthetic incident.
func (e *Engine) AddMassFailure(inc *model.Incident) bool {
	e.mu.Lock()
	if e.frozen || inc == nil {
		e.mu.Unlock()
		return false
	}
	key := inc.Key
	if _, exists := e.massFailures[key]; exists {
		e.mu.Unlock()
		return false
	}
	stored := inc.Clone()
	stored.ID = incidentID(key)
	stored.FirstSeen = e.now()
	stored.LastSeen = stored.FirstSeen
	if stored.State != model.StateResolved {
		stored.State = model.StateActive
	}
	stored.NotifiedSig = notifSig(stored)
	e.massFailures[key] = stored
	e.dirty = true
	snapshot := stored.Clone()
	e.mu.Unlock()

	e.emit(transition{snapshot, model.ActionCreate})
	return true
}

// RemoveMassFailure resolves the synthetic incident and releases suppressed
// symptoms so an ongoing incident can be announced again.
func (e *Engine) RemoveMassFailure(key model.IncidentKey) bool {
	e.mu.Lock()
	if e.frozen {
		e.mu.Unlock()
		return false
	}
	inc, exists := e.massFailures[key]
	if !exists {
		e.mu.Unlock()
		return false
	}
	delete(e.massFailures, key)
	resolved := inc.Clone()
	resolved.State = model.StateResolved
	resolved.NotifiedSig = notifSig(resolved)
	released := e.releaseSuppressedLocked(key)
	e.mu.Unlock()

	e.emit(append(
		[]transition{{resolved, model.ActionResolved}}, released...,
	)...)
	return true
}
