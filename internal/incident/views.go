package incident

import (
	"sort"

	"github.com/abahmed/kwatch/internal/model"
)

func (e *Engine) ActiveCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, inc := range e.state {
		if inc.State != model.StateResolved {
			n++
		}
	}
	return n
}

func (e *Engine) Snapshot() []model.IncidentView {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.IncidentView, 0, len(e.state))
	for _, inc := range e.state {
		out = append(out, model.IncidentView{
			Key: inc.Key, Reason: inc.Reason, Namespace: inc.Namespace,
			Name: inc.Name, State: inc.State, Severity: inc.Severity,
			Count: inc.Count, FirstSeen: inc.FirstSeen, LastSeen: inc.LastSeen,
			Hint: inc.Hint,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (e *Engine) GetLastContainerState(
	namespace, podName, container string,
) *model.ContainerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := lastContainerKey(namespace, podName, container)
	entry, ok := e.lastContainerIndex[key]
	if !ok || entry.state == nil {
		return nil
	}
	state := *entry.state
	return &state
}

// SnapshotAll returns a deep copy of all non-resolved incidents keyed by key.
// It consumes the dirty flag; use ActiveIncidents for a read-only snapshot.
func (e *Engine) SnapshotAll() map[model.IncidentKey]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.dirty {
		return nil
	}
	out := make(map[model.IncidentKey]*model.Incident, len(e.state))
	for key, inc := range e.state {
		if inc.State != model.StateResolved {
			out[key] = inc.Clone()
		}
	}
	e.dirty = false
	return out
}

// ActiveIncidents returns a deep copy without consuming the dirty flag.
func (e *Engine) ActiveIncidents() map[model.IncidentKey]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[model.IncidentKey]*model.Incident, len(e.state))
	for key, inc := range e.state {
		if inc.State != model.StateResolved {
			out[key] = inc.Clone()
		}
	}
	return out
}
