package incident

import (
	"slices"
	"strings"
)

// The problem title is the stable headline of an incident: what is
// failing, in the composer's words of an announcement, never the words
// of an update, a recovery or a move. The pipeline writes it when the
// incident opens, before any message is sent, and again when the open
// problem gets worse (a new cause, a new root, new failure reasons); a
// resolved incident keeps the last one. The dashboard names
// incidents by it, so an incident nobody was told about has a title too.
// A recovery never re-titles it (see noteProblem).

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

// noteProblem queues p for a new problem title when the problem got
// worse: another cause, another root, or a failure reason the title was
// not written for. Members that only drop out (recoveries) never
// re-title it: the last one left, such as an event about how a killed
// container stopped, is not what the incident is about. An incident
// titled in an earlier session (restored, so without the key it was
// titled for) takes its current key as the baseline. Only a live
// incident with a failing member is titled: the composer has nothing to
// say about one without.
func (m *Manager) noteProblem(p *Incident) {
	if p.State == Resolved || !hasFailingMember(p) {
		return
	}
	key := problemKey(p)
	if p.problemKey == "" && p.Problem != "" {
		p.problemKey = key
		return
	}
	if !worseProblem(p.problemKey, key) {
		return
	}
	if m.untitled == nil {
		m.untitled = map[string]bool{}
	}
	m.untitled[p.ID] = true
}

// worseProblem reports whether the problem of key next is worse than
// the one titled for key titled: the root changed, a cause appeared or
// changed (not one that went away), or a failure reason is new.
func worseProblem(titled, next string) bool {
	if titled == next {
		return false
	}
	was, now := strings.Split(titled, "|"), strings.Split(next, "|")
	if len(was) != 4 || len(now) != 4 {
		return true
	}
	if was[0] != now[0] || (now[1] != "" && was[1] != now[1]) {
		return true
	}
	reasons := strings.Split(was[3], ",")
	for _, reason := range strings.Split(now[3], ",") {
		if reason != "" && !slices.Contains(reasons, reason) {
			return true
		}
	}
	return false
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

// SetProblem stores the problem title of incident id, bounded for a
// list, and full, the same title uncut. An empty title keeps the earlier
// ones. No-op for an unknown incident.
func (m *Manager) SetProblem(id, title, full string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && title != "" {
		p.Problem, p.ProblemFull = title, full
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

// FullTitle is Title uncut, the problem title as the composer wrote
// it; a record saved before it existed has its bounded Title.
func (p Incident) FullTitle() string {
	if p.ProblemFull != "" {
		return p.ProblemFull
	}
	return p.Title()
}
