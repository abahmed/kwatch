package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// timeValue is an optional time: the zero value is "not known".
type timeValue struct {
	t  time.Time
	ok bool
}

// earlier returns the earlier of the value and t.
func (tv timeValue) earlier(t time.Time) timeValue {
	if !tv.ok || t.Before(tv.t) {
		return timeValue{t: t, ok: true}
	}
	return tv
}

// candidateStart is when the candidate went wrong; see startOf.
func (v *view) candidateStart(c *candidate) timeValue {
	return v.startOf(c.id)
}

// startOf is when id went wrong: its earliest unhealthy finding or its
// earliest change in the window. A zone or node pool has no findings
// of its own; it went wrong when its first node broke.
func (v *view) startOf(id inventory.EntityID) timeValue {
	start := v.earliestSince([]inventory.EntityID{id})
	if id.Kind == kube.KindZone || id.Kind == kube.KindNodePool {
		start = v.groupStart(id)
	}
	for _, change := range v.changesOf(id) {
		if changeMode(change) != ModeScaled {
			start = start.earlier(change.At)
		}
	}
	return start
}

// groupStart is the earliest failing finding among the broken nodes of
// a zone or pool.
func (v *view) groupStart(group inventory.EntityID) (start timeValue) {
	for _, node := range v.s.Model.Related(
		group, inventory.PartOf, inventory.Incoming,
	) {
		for _, f := range v.s.Findings[node] {
			if f.Health == detection.Failing && !f.Since.IsZero() {
				start = start.earlier(f.Since)
			}
		}
	}
	return start
}

// began is when the candidate went wrong, as a time for the incident
// layer; zero when unknown. See startOf.
func (v *view) began(id inventory.EntityID) time.Time {
	start := v.startOf(id)
	if !start.ok || v.observedAtStart(start.t) {
		return time.Time{}
	}
	return start.t
}

// causeBegan is when the candidate went wrong, as a time for the incident
// layer; zero when unknown. A pseudo cause, an entity the model does not
// hold (a finalizer, a scheduling constraint, a shared error), exists only
// through the failures it explains, so it began when the first of them
// did. Without a start the incident layer could not refuse it as a late
// takeover of an incident announced long before.
func (v *view) causeBegan(c *candidate) time.Time {
	if began := v.began(c.id); !began.IsZero() ||
		v.s.Model.Exists(c.id) {
		return began
	}
	first := v.earliestSince(c.others())
	if !first.ok || v.observedAtStart(first.t) {
		return time.Time{}
	}
	return first.t
}

// observedAtStart reports a start at the moment kwatch began: after a
// restart, findings first seen in its first sync carry the observation
// time, so the moment says when kwatch noticed, not when the problem
// began. Only a start within TemporalSlack of it counts; a later start
// is a real time, from the cluster or seen live.
func (v *view) observedAtStart(t time.Time) bool {
	floor := v.s.Started
	return !floor.IsZero() && !t.Before(floor.Add(-TemporalSlack)) &&
		!t.After(floor.Add(TemporalSlack))
}

// predatesStart reports a time from before kwatch began observing.
func (v *view) predatesStart(t time.Time) bool {
	floor := v.s.Started
	return !floor.IsZero() && t.Before(floor.Add(-TemporalSlack))
}

// scoreTemporal checks that the cause began before its effects. A
// cause that began well after the failures cannot have caused them.
// A workload blamed for itself says nothing here: its own summary
// ("0 of 2 replicas are ready") starts with the failures it restates.
func scoreTemporal(v *view, c *candidate) outcome {
	effects := c.others()
	if len(effects) == 0 || c.isSelf() {
		return outcome{}
	}
	cause := v.candidateStart(c)
	first := v.earliestSince(effects)
	if !cause.ok || !first.ok {
		return outcome{}
	}
	// A cause stamped at observation time compared with a failure the
	// cluster dated before kwatch started mixes two clocks: the cause
	// only looks late. Later failures were seen live, so the order
	// holds.
	if v.observedAtStart(cause.t) && v.predatesStart(first.t) {
		return outcome{}
	}
	if cause.t.After(first.t.Add(TemporalSlack)) {
		return outcome{weight: -TemporalAfter,
			code: rootcause.ProofBeganAfter,
			text: "it began after the failures it would explain"}
	}
	return outcome{weight: TemporalBefore,
		code: rootcause.ProofBeganBefore,
		text: "it began before the failures"}
}
