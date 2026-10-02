package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
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

// candidateStart is when the candidate went wrong: its earliest
// unhealthy finding or its earliest change in the window.
func (v *view) candidateStart(c *candidate) timeValue {
	start := v.earliestSince([]inventory.EntityID{c.id})
	for _, change := range v.changesOf(c.id) {
		if changeMode(change) != ModeScaled {
			start = start.earlier(change.At)
		}
	}
	return start
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
