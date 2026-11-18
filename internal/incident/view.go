package incident

import (
	"cmp"
	"slices"

	"github.com/abahmed/kwatch/internal/detection"
)

// View is one published reading of every incident: what the dashboard
// shows. The decision loop builds it once per iteration (Publish) and
// swaps it in whole; nothing changes it after, so every reader of one
// View sees the same revision, and a reader never waits for the
// manager's lock. Callers must treat what it returns as read-only: it
// is shared by every reader.
type View struct {
	// Revision numbers the publications of one manager, from 1.
	Revision  uint64
	incidents []Incident
	byID      map[string]int
	old       []detection.Finding
	oldDays   int
}

// NewView builds a view of incidents, ordered by ID, with the old
// problems not shown in them and the days that make a problem old.
// The manager builds its own; tests build one directly.
func NewView(
	revision uint64, incidents []Incident,
	old []detection.Finding, oldDays int,
) *View {
	v := &View{Revision: revision,
		incidents: slices.Clip(slices.Clone(incidents)),
		byID:      make(map[string]int, len(incidents)),
		old:       slices.Clip(slices.Clone(old)), oldDays: oldDays}
	slices.SortStableFunc(v.incidents, func(a, b Incident) int {
		return cmp.Compare(a.ID, b.ID)
	})
	for i, p := range v.incidents {
		v.byID[p.ID] = i
	}
	return v
}

// Incidents are every incident of the view, by ID. Read-only.
func (v *View) Incidents() []Incident {
	return v.incidents
}

// Incident is the incident with the given ID, or nil. Read-only.
func (v *View) Incident(id string) *Incident {
	i, ok := v.byID[id]
	if !ok {
		return nil
	}
	return &v.incidents[i]
}

// OldProblems are the old low findings that are not an issue, by
// object then reason: none of their objects is on the issues list.
// Read-only.
func (v *View) OldProblems() []detection.Finding {
	return v.old
}

// OldProblemDays is the configured age of an old problem in whole
// days; zero when old problems are off.
func (v *View) OldProblemDays() int {
	return v.oldDays
}

// Publish builds the view of the incidents as they are now and makes
// it the one View returns. The decision loop calls it once per
// iteration, so readers cost nothing per request.
func (m *Manager) Publish() *View {
	m.mu.Lock()
	m.revision++
	v := &View{Revision: m.revision,
		incidents: m.snapshots(),
		old:       slices.Clip(m.oldProblems()),
		oldDays:   m.OldProblemDays()}
	m.mu.Unlock()
	v.byID = make(map[string]int, len(v.incidents))
	for i, p := range v.incidents {
		v.byID[p.ID] = i
	}
	m.view.Store(v)
	return v
}

// View is the latest published view; nil before the first Publish.
// It takes no lock.
func (m *Manager) View() *View {
	return m.view.Load()
}

// snapshots are detached copies of every incident ordered by ID, with
// their relations. The caller holds m.mu.
func (m *Manager) snapshots() []Incident {
	out := make([]Incident, 0, len(m.incidents))
	index := m.indexLive()
	for _, id := range m.sortedIDs() {
		snap := m.incidents[id].Snapshot()
		m.relateAmong(&snap, index)
		out = append(out, snap)
	}
	return out
}
