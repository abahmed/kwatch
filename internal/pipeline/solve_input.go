package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// movedSet collects entities, each once, in the order they first moved.
// The zero value is empty and ready to use.
type movedSet struct {
	order []inventory.EntityID
	seen  map[inventory.EntityID]bool
}

func (m *movedSet) add(id inventory.EntityID) {
	if m.seen[id] {
		return
	}
	if m.seen == nil {
		m.seen = map[inventory.EntityID]bool{}
	}
	m.seen[id] = true
	m.order = append(m.order, id)
}

// len is the number of entities collected.
func (m *movedSet) len() int {
	return len(m.order)
}

// take returns the entities in order and empties the set.
func (m *movedSet) take() []inventory.EntityID {
	out := m.order
	m.order = nil
	clear(m.seen)
	return out
}

// moves reports whether an observation can change a root cause
// without changing any finding: a new or removed relation, a new note,
// a recorded change, or an object that is gone. Attribute updates are
// left out: status churn is not a change, and a status that matters
// changes a finding. It reads the model before the observation is
// applied.
func (e *Engine) moves(o inventory.Observation) bool {
	switch o.Kind {
	case inventory.Changed, inventory.Gone:
		return true
	case inventory.Related:
		current := e.deps.Model.Related(
			o.Entity, o.Relation, inventory.Outgoing)
		return !sameTargets(current, o.Targets)
	case inventory.Noted:
		return !e.knownNote(o)
	}
	return false
}

// movedWith are the entities that also move when o's entity does. A
// NetworkPolicy decides which calls in its namespace go through, and a
// failing pod's walk is gated on its namespace (see explain's
// policyCallHops), so a policy created or edited later moves it.
func movedWith(o inventory.Observation) []inventory.EntityID {
	if o.Entity.Kind != kube.KindNetworkPolicy || o.Entity.Namespace == "" {
		return nil
	}
	return []inventory.EntityID{inventory.CoreID(kube.KindNamespace, "",
		o.Entity.Namespace)}
}

// knownNote reports whether the entity already holds a note with the
// same reason and message: a repeated event says nothing new.
func (e *Engine) knownNote(o inventory.Observation) bool {
	for _, note := range e.deps.Model.Notes(o.Entity, time.Time{}) {
		if note.Reason == o.Note.Reason && note.Message == o.Note.Message {
			return true
		}
	}
	return false
}

// sameTargets compares two target lists as sets.
func sameTargets(a, b []inventory.EntityID) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[inventory.EntityID]bool, len(a))
	for _, id := range a {
		seen[id] = true
	}
	for _, id := range b {
		if !seen[id] {
			return false
		}
	}
	return true
}

// keepFindings copies the tracker's active findings of id.
func (e *Engine) keepFindings(id inventory.EntityID) {
	active := e.tracker.Active(id)
	if len(active) > 0 {
		e.findings[id] = active
	} else {
		delete(e.findings, id)
	}
	e.published.set(id, active)
}

// activeFindings returns a shallow copy of the active findings, so the
// snapshot never changes under a solve.
func (e *Engine) activeFindings() map[inventory.EntityID][]detection.Finding {
	out := make(map[inventory.EntityID][]detection.Finding, len(e.findings))
	for id, found := range e.findings {
		out[id] = found
	}
	return out
}

// judgedThrough are the relations whose source is judged by reading
// its target: an admission webhook or an APIService is healthy only
// while the Service it calls has ready endpoints.
var judgedThrough = []inventory.RelationType{inventory.Serves}

// markDependents queues, for the next step, the entities judged
// through an entity whose findings moved, so a Service losing its
// endpoints also re-evaluates the webhooks that call it.
func (e *Engine) markDependents(transitions []detection.Transition) {
	seen := map[inventory.EntityID]bool{}
	for _, t := range transitions {
		id := t.Finding.Entity
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, relation := range judgedThrough {
			e.dirty = append(e.dirty, e.deps.Model.Related(
				id, relation, inventory.Incoming)...)
		}
	}
}

// referrers are the relations whose source reads the target when it is
// judged: an Ingress is judged by the Service it routes to, a workload
// by the ConfigMaps and Secrets it references.
var referrers = []inventory.RelationType{
	inventory.RoutesTo, inventory.References, inventory.Calls,
}

// readers returns the entities, other than the observed one, whose
// detectors read the observed entity and so must run again: the Service
// an EndpointSlice backs, and, when an object is gone or appears,
// everything that routes to or references it (an Ingress whose missing
// TLS Secret is created must clear its finding at once). The model only
// touches the entity itself and the targets of its own relations.
func (e *Engine) readers(
	o inventory.Observation, update inventory.Update,
) []inventory.EntityID {
	out := e.deps.Model.Related(o.Entity, inventory.Backs, inventory.Outgoing)
	if o.Kind == inventory.Noted && detectors.IsQuotaRefusal(o.Note) {
		// The refusal is recorded on the controller, yet it explains the
		// quota: judge the namespace quotas again so the one named in the
		// message becomes the cause of the missing pods.
		out = append(out, e.deps.Model.EntitiesIn(
			kube.KindQuota, o.Entity.Namespace)...)
	}
	if o.Entity.Kind == kube.KindService {
		// A Service edit can add or remove the port an Ingress names.
		out = append(out, e.deps.Model.Related(
			o.Entity, inventory.RoutesTo, inventory.Incoming)...)
	}
	out = append(out, e.preemptedPods(o.Entity)...)
	if o.Kind != inventory.Gone && !update.Appeared {
		return out
	}
	for _, relation := range referrers {
		out = append(out, e.deps.Model.Related(
			o.Entity, relation, inventory.Incoming)...)
	}
	return out
}

// activeAdvisories lists the configuration risks among the active
// findings, for the digest to name once each.
func (e *Engine) activeAdvisories() []detection.Finding {
	var out []detection.Finding
	for _, found := range e.findings {
		for _, f := range found {
			if f.Advisory {
				out = append(out, f)
			}
		}
	}
	return out
}
