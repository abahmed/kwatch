package incident

import (
	"slices"
	"strings"
)

// The problem title is the stable headline of an incident: what is
// failing, in the composer's words of an announcement, never the words
// of an update, a recovery or a move. The pipeline writes it when the
// incident opens, before any message is sent, and again when the open
// problem changes materially (a new cause, another failure mode, other
// reasons); a resolved incident keeps the last one. The dashboard names
// incidents by it, so an incident nobody was told about has a title too.

// problemKey is what the problem title depends on: the root, the cause,
// the failure mode and the reasons of the failing members. Counts are
// left out, so a storm that grows by a pod does not rewrite the title.
func problemKey(p *Incident) string {
	reasons := make([]string, 0, len(p.Members))
	for key, s := range p.Members {
		if !s.Advisory && !slices.Contains(reasons, key.Reason) {
			reasons = append(reasons, key.Reason)
		}
	}
	slices.Sort(reasons)
	cause := ""
	if p.Cause != nil {
		cause = p.Cause.Root.String() + "/" + p.Cause.Rule
	}
	return strings.Join([]string{p.Root.String(), cause, string(p.Mode),
		strings.Join(reasons, ",")}, "|")
}

// noteProblem queues p for a new problem title when what it depends on
// changed. Only a live incident with a failing member is titled: the
// composer has nothing to say about one without.
func (m *Manager) noteProblem(p *Incident) {
	if p.State == Resolved || !hasFailingMember(p) {
		return
	}
	if key := problemKey(p); key != p.problemKey {
		if m.untitled == nil {
			m.untitled = map[string]bool{}
		}
		m.untitled[p.ID] = true
	}
}

// TakeUntitled returns copies of the incidents whose problem title is
// due, and forgets them: each is titled once per material change.
func (m *Manager) TakeUntitled() []Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Incident
	for _, id := range m.sortedIDs() {
		if !m.untitled[id] {
			continue
		}
		p := m.incidents[id]
		if p.State == Resolved || !hasFailingMember(p) {
			continue
		}
		p.problemKey = problemKey(p)
		out = append(out, p.Snapshot())
	}
	clear(m.untitled)
	return out
}

// SetProblem stores the problem title of incident id. An empty title
// keeps the earlier one. No-op for an unknown incident.
func (m *Manager) SetProblem(id, title string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && title != "" {
		p.Problem = title
	}
}

// Title is the incident's problem title, or, for a record saved before
// problem titles, the headline of its last message; empty when neither
// is known.
func (p Incident) Title() string {
	if p.Problem != "" {
		return p.Problem
	}
	return p.Headline
}
