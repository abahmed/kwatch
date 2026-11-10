package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// The causal window answers "what changed shortly before the failure".
// It is measured back from when the failure began, never from now: a
// crash loop that outlasts the window was still caused by the edit made
// just before it started, and a re-solve an hour in must find the same
// cause as the first one.
//
// A change of id therefore counts from the earliest onset, less the
// window, among the failures whose walk reached id (see noteReach).
// The onset of a failure is that of its unit (a pod and its
// containers fail as one), widened by the failures of the same
// workload that fail the same way: a replica replaced during a long
// outage continues the failure of the one it replaced, and the old
// change is not too far from it. A replica of the same workload that
// fails another way (one Pending for a day beside a fresh crash loop)
// is another failure and lends nothing. An entity no failure reached
// keeps the window that ends now.

// MaxChangeAge bounds how far back of now a change can be a cause,
// however long the failure has lasted: a change older than a week is
// history, not the cause of what is failing now.
const MaxChangeAge = 7 * 24 * time.Hour

// historyFrom is how far back the change history is read: the causal
// window of the earliest failure and what a run's exposure adds to it
// (see exposureFrom), but never more than MaxChangeAge before now.
func (v *view) historyFrom() time.Time {
	from := v.since
	for _, failure := range failures(v.s.Findings) {
		onset := v.earliestSince([]inventory.EntityID{failure})
		if onset.ok && onset.t.Before(v.s.Now) {
			if start := onset.t.Add(-v.s.window()); start.Before(from) {
				from = start
			}
		}
	}
	from = from.Add(-ExposureWindow)
	if limit := v.s.Now.Add(-MaxChangeAge); from.Before(limit) {
		return limit
	}
	return from
}

// groupOnset is when the failure of effect began: the earliest
// unhealthy finding of its unit or of a failure of the same workload
// that shares one of the unit's modes.
func (v *view) groupOnset(effect inventory.EntityID) timeValue {
	unit := v.failureUnit(effect)
	if onset, ok := v.onsets[unit[0]]; ok {
		return onset
	}
	if v.onsets == nil {
		v.onsets = map[inventory.EntityID]timeValue{}
		v.anchored = map[inventory.EntityID][]inventory.EntityID{}
		for _, failure := range failures(v.s.Findings) {
			a := v.anchorOf(failure)
			v.anchored[a] = append(v.anchored[a], failure)
		}
	}
	onset := v.earliestSince(unit)
	modes := v.unhealthyModes(unit)
	for _, failure := range v.anchored[v.anchorOf(effect)] {
		for _, f := range v.s.Findings[failure] {
			if unhealthy(f) && modes[f.Mode] && !f.Since.IsZero() {
				onset = onset.earlier(f.Since)
			}
		}
	}
	v.onsets[unit[0]] = onset
	return onset
}

// unhealthyModes is the set of modes of the unhealthy findings of ids.
func (v *view) unhealthyModes(
	ids []inventory.EntityID,
) map[detection.Mode]bool {
	modes := map[detection.Mode]bool{}
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			if unhealthy(f) {
				modes[f.Mode] = true
			}
		}
	}
	return modes
}

// failureUnit is effect with the entities that fail as one with it: a
// pod and its containers. The first entry keys the unit.
func (v *view) failureUnit(effect inventory.EntityID) []inventory.EntityID {
	pod, ok := v.podOf(effect)
	if !ok {
		return []inventory.EntityID{effect}
	}
	return append([]inventory.EntityID{pod}, v.s.Model.Related(
		pod, inventory.PartOf, inventory.Incoming)...)
}

// windowStart is where the causal window of effect begins: the window
// before its anchor began to fail. A failure without a known start, or
// one dated after now, keeps the window that ends now.
func (v *view) windowStart(effect inventory.EntityID) time.Time {
	onset := v.groupOnset(effect)
	if !onset.ok || onset.t.After(v.s.Now) {
		return v.since
	}
	return onset.t.Add(-v.s.window())
}

// noteReach records that the walk from effect reached each of ids, and
// with them their owners: a change of any of them counts from effect's
// window on.
func (v *view) noteReach(effect inventory.EntityID, reached []reach) {
	start := v.windowStart(effect)
	if !start.Before(v.since) {
		return
	}
	ids := append([]inventory.EntityID{effect},
		rootcause.OwnerChain(v.s.Model, effect)...)
	if pod, ok := v.podOf(effect); ok {
		ids = append(append(ids, pod),
			rootcause.OwnerChain(v.s.Model, pod)...)
	}
	for _, r := range reached {
		ids = append(append(ids, r.id),
			rootcause.OwnerChain(v.s.Model, r.id)...)
	}
	for _, id := range ids {
		if known, ok := v.reachFrom[id]; !ok || start.Before(known) {
			v.reachFrom[id] = start
		}
	}
}

// tooOldFor reports a change of id made before the causal window of
// effect began, and not kept by a run's exposure either: it is history
// for this failure even when another, older failure keeps it in view.
func (v *view) tooOldFor(
	id, effect inventory.EntityID, change inventory.Change,
) bool {
	if !change.At.Before(v.windowStart(effect)) {
		return false
	}
	exposed, ok := v.exposureFrom(id)
	return !ok || change.At.Before(exposed)
}
