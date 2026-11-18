package incident

import (
	"hash/fnv"
	"slices"
	"strconv"
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

// problemKey is what the problem title depends on: the root, the
// cause, the failure mode, the reasons of the failing members and the
// evidence that names what they are about. Counts are left out, so a
// storm that grows by a pod does not rewrite the title.
func problemKey(p *Incident) string {
	var reasons, named []string
	for key, s := range p.Members {
		if s.Advisory {
			continue
		}
		reasons = appendNew(reasons, key.Reason)
		for _, e := range s.Evidence {
			if e.Value != "" {
				named = appendNew(named, key.Reason+":"+e.Label)
			}
		}
	}
	slices.Sort(reasons)
	slices.Sort(named)
	return strings.Join([]string{p.Root.String(), causeKey(p),
		string(p.Mode), strings.Join(reasons, ","),
		strings.Join(named, ",")}, "|")
}

// causeKey names the cause the title and steps are written for: its
// root, row and mode; "" without a cause.
func causeKey(p *Incident) string {
	if p.Cause == nil {
		return ""
	}
	return p.Cause.Root.String() + "/" + p.Cause.Rule + "/" +
		string(p.Cause.Mode)
}

// appendNew appends s unless list holds it.
func appendNew(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// noteProblem queues p for a new problem title when the diagnosis got
// worse or more specific: another root, another cause (not one that
// went away), a failure reason or a piece of naming evidence the title
// was not written for, or a reshape the key does not show (Pending.
// reshaped: a member moved out, a refuted cause dropped). Members that
// only drop out (recoveries) never re-title it: the last one left,
// such as an event about how a killed container stopped, is not what
// the incident is about. An incident titled in an earlier session
// (restored, so without the key it was titled for) takes its current
// key as the baseline. Only a live incident with a failing member is
// titled: the composer has nothing to say about one without.
func (m *Manager) noteProblem(p *Incident) {
	if p.State == Resolved || !hasFailingMember(p) {
		return
	}
	key := problemKey(p)
	if p.problemKey == "" && p.Problem != "" && !p.Pending.reshaped {
		p.problemKey = key
		return
	}
	if !p.Pending.reshaped && !worseProblem(p.problemKey, key) {
		return
	}
	if m.untitled == nil {
		m.untitled = map[string]bool{}
	}
	m.untitled[p.ID] = true
}

// worseProblem reports whether the problem of key next is worse than
// the one titled for key titled: the root changed, a cause appeared or
// changed (not one that went away), or a failure reason or naming
// evidence is new.
func worseProblem(titled, next string) bool {
	if titled == next {
		return false
	}
	was, now := strings.Split(titled, "|"), strings.Split(next, "|")
	if len(was) != len(now) {
		return true
	}
	if was[0] != now[0] || (now[1] != "" && was[1] != now[1]) {
		return true
	}
	for _, part := range []int{3, 4} {
		before := strings.Split(was[part], ",")
		for _, item := range strings.Split(now[part], ",") {
			if item != "" && !slices.Contains(before, item) {
				return true
			}
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
		p.Pending.reshaped = false
		out = append(out, p.Snapshot())
	}
	clear(m.untitled)
	return out
}

// StepsStale reports steps written for another cause or other failing
// members than the incident has now: they are recomputed with its
// title, so a revised cause or a member that moved out does not leave
// steps about what is no longer the story.
func (p Incident) StepsStale() bool {
	return p.Pending.stepsFor != stepsKey(&p)
}

// stepsKey is what the steps are written for: the cause and the
// failing members, since steps name the objects to look at.
func stepsKey(p *Incident) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(causeKey(p)))
	ids := make([]string, 0, len(p.Members))
	for key, f := range p.Members {
		if !f.Advisory {
			ids = appendNew(ids, key.Entity.String())
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		_, _ = h.Write([]byte("|" + id))
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

// SetSteps stores the steps written for the incident's current cause
// and members.
// No steps keep the earlier ones. No-op for an unknown incident.
func (m *Manager) SetSteps(id string, steps []FixStep) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && len(steps) > 0 {
		p.Steps = boundSteps(steps)
		p.Pending.stepsFor = stepsKey(p)
	}
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
