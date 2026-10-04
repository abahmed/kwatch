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
	if !start.ok {
		return time.Time{}
	}
	return start.t
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
	if cause.t.After(first.t.Add(TemporalSlack)) {
		return outcome{weight: -TemporalAfter,
			code: rootcause.ProofBeganAfter,
			text: "it began after the failures it would explain"}
	}
	return outcome{weight: TemporalBefore,
		code: rootcause.ProofBeganBefore,
		text: "it began before the failures"}
}
