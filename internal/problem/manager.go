package problem

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

// maxRootDepth bounds recursive root resolution.
const maxRootDepth = 4

// Manager owns every problem. It is safe for concurrent use.
type Manager struct {
	mu       sync.Mutex
	cfg      Config
	engine   *reason.Engine
	problems map[string]*Problem
	byMember map[signal.Key]string
	// restoreGrace delays recovery of restored problems until detectors
	// had time to re-raise their signals.
	restoreGrace time.Time
}

// NewManager builds a manager.
func NewManager(cfg Config, engine *reason.Engine) *Manager {
	return &Manager{
		cfg: cfg.withDefaults(), engine: engine,
		problems: make(map[string]*Problem),
		byMember: make(map[signal.Key]string),
	}
}

// Apply folds signal transitions into problems. Raised and changed
// signals are explained and attached to their root's problem; cleared
// signals are detached.
func (m *Manager) Apply(q reason.Query, transitions []signal.Transition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range transitions {
		switch t.Kind {
		case signal.Raised, signal.Changed:
			m.attach(q, t.Signal)
		case signal.Cleared:
			m.detach(q.Now, t.Signal)
		}
	}
}

func (m *Manager) attach(q reason.Query, s signal.Signal) {
	root, cause := m.resolveRoot(q, s)
	id := root.String()
	key := s.Key()
	if previous, ok := m.byMember[key]; ok && previous != id {
		m.detach(q.Now, s)
		m.problem(q.Now, id, root).note(q.Now,
			"cause revised: now explained by "+describe(root))
	}
	p := m.problem(q.Now, id, root)
	if _, known := p.Members[key]; !known {
		p.note(q.Now, s.Summary+" ("+describe(s.Entity)+")")
	}
	p.Members[key] = s
	if cause != nil {
		p.Cause = cause
	}
	m.byMember[key] = id
	p.Impact = impact(q.Model, p)
	p.Tier = tier(p)
}

// resolveRoot follows explanations until the root has no better cause, so
// a Service symptom explained by a Deployment whose pods fail because of a
// node ends at the node.
func (m *Manager) resolveRoot(
	q reason.Query, s signal.Signal,
) (knowledge.EntityID, *reason.Hypothesis) {
	var cause *reason.Hypothesis
	current := s
	for depth := 0; depth < maxRootDepth; depth++ {
		explanation := m.engine.Explain(q, current)
		if explanation.Cause == nil {
			break
		}
		cause = explanation.Cause
		next, ok := deeperSignal(q, cause)
		if !ok {
			break
		}
		current = next
	}
	own := defaultRoot(q.Model, s.Entity)
	if cause == nil {
		return own, nil
	}
	root := defaultRoot(q.Model, cause.Root)
	// An explanation that stays inside the symptom's own workload (a
	// Deployment explained by its crashing pods) is not a cause.
	if root == own && cause.Change == nil {
		return own, nil
	}
	return root, cause
}

// deeperSignal picks a signal of the cause's root, or of the failing pods
// it owns, to explain further.
func deeperSignal(
	q reason.Query, cause *reason.Hypothesis,
) (signal.Signal, bool) {
	if cause.Change != nil {
		return signal.Signal{}, false
	}
	if len(cause.RootSignals) > 0 {
		return cause.RootSignals[0], true
	}
	for _, pod := range ownedPods(q.Model, cause.Root) {
		if active := q.Signals.Active(pod); len(active) > 0 {
			return active[0], true
		}
		for _, container := range q.Model.Related(
			pod, knowledge.PartOf, knowledge.Incoming,
		) {
			if active := q.Signals.Active(container); len(active) > 0 {
				return active[0], true
			}
		}
	}
	return signal.Signal{}, false
}

func (m *Manager) detach(now time.Time, s signal.Signal) {
	key := s.Key()
	id, ok := m.byMember[key]
	if !ok {
		return
	}
	delete(m.byMember, key)
	if p := m.problems[id]; p != nil {
		delete(p.Members, key)
		p.note(now, "recovered: "+s.Summary+" ("+describe(s.Entity)+")")
	}
}

// problem returns the live problem for id, reopening a recently resolved
// one so recurrence is visible instead of starting from nothing.
func (m *Manager) problem(
	now time.Time, id string, root knowledge.EntityID,
) *Problem {
	p := m.problems[id]
	switch {
	case p == nil:
		p = &Problem{
			ID: id, Root: root, State: Settling, Opened: now,
			Members: make(map[signal.Key]signal.Signal),
		}
		m.problems[id] = p
	case p.State == Resolved:
		p.State, p.Opened, p.Revision, p.Digest = Settling, now, 0, ""
		p.note(now, "happened again")
	}
	return p
}

func (p *Problem) note(at time.Time, text string) {
	const maxTimeline = 50
	p.Timeline = append(p.Timeline, Event{At: at, Text: text})
	if over := len(p.Timeline) - maxTimeline; over > 0 {
		p.Timeline = append(p.Timeline[:0:0], p.Timeline[over:]...)
	}
}

func describe(id knowledge.EntityID) string {
	if id.Namespace == "" {
		return string(id.Kind) + " " + id.Name
	}
	return string(id.Kind) + " " + id.Namespace + "/" + id.Name
}
