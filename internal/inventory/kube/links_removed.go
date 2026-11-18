package kube

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// The model keeps only the relations that hold now, so a dependency
// removed just before a failure would leave no trace. LinkJournal turns
// the end of a link between long-lived objects into a change of the
// object that held it ("Service payments stopped routing to deployment
// api"), so the rules and the list of recent changes can name it.
//
// Only links people configure are recorded:
//
//   - a Service's routing to a workload, and a NetworkPolicy's coverage
//     of one: the workloads whose pod template labels the selector
//     matches (relations routes-to and selects, read at workload level,
//     never per pod);
//   - a workload's use of a ConfigMap, Secret or volume claim, mounted
//     or named in its environment (references);
//   - an Ingress or route sending traffic to a Service (routes-to), and
//     a webhook, APIService or conversion served by one (serves);
//   - a configured dependency (calls) of anything but a pod, and, for a
//     workload, a dependency its pod template stops naming (the
//     template.calls attribute, so only a change of the template ends
//     it, never pods coming and going).
//
// Links of pods, containers and node assignment are never recorded:
// they end on every restart. Nor is a link whose holder was deleted:
// the deletion is the change. A link that comes back within
// LinkFlapWindow records nothing.

// LinkFlapWindow is how long a removed link must stay gone before its
// end is recorded.
const LinkFlapWindow = time.Minute

// maxPendingLinks bounds the removed links waiting out LinkFlapWindow.
// Recorded ones count against the model's change history caps.
const maxPendingLinks = 256

// heldLink is one directed relation an object holds.
type heldLink struct {
	from, to inventory.EntityID
	rel      inventory.RelationType
}

// pendingLink is a link that ended at at and has not come back.
type pendingLink struct {
	heldLink
	at time.Time
}

// LinkJournal records the end of configured links. It is owned by the
// goroutine that applies observations and is not safe for concurrent
// use.
type LinkJournal struct {
	pending []pendingLink
}

// NewLinkJournal returns an empty journal.
func NewLinkJournal() *LinkJournal { return &LinkJournal{} }

// LinkMark is what Before read, for After to compare.
type LinkMark struct {
	links []heldLink
	read  bool
}

// Before reads the configured links o may end. Call it before o is
// applied to the model and hand the mark to After.
func (j *LinkJournal) Before(
	r inventory.Reader, o inventory.Observation,
) LinkMark {
	if !linkObservation(o) {
		return LinkMark{}
	}
	return LinkMark{links: linksOf(r, o), read: true}
}

// After compares the links of o now with mark: the ended ones wait out
// LinkFlapWindow, and a link that came back is no longer pending.
func (j *LinkJournal) After(
	r inventory.Reader, o inventory.Observation, mark LinkMark,
) {
	if !mark.read {
		return
	}
	now := linksOf(r, o)
	for _, l := range now {
		j.forget(l)
	}
	for _, l := range mark.links {
		if !hasLink(now, l) && !j.waiting(l) &&
			len(j.pending) < maxPendingLinks {
			j.pending = append(j.pending, pendingLink{heldLink: l, at: o.At})
		}
	}
}

// Due returns the changes of the links that stayed gone for
// LinkFlapWindow by now, as observations to apply, and forgets them. A
// link whose holder is gone, or that holds again, records nothing.
func (j *LinkJournal) Due(
	r inventory.Reader, now time.Time,
) []inventory.Observation {
	var out []inventory.Observation
	kept := j.pending[:0]
	for _, p := range j.pending {
		switch {
		case now.Before(p.at.Add(LinkFlapWindow)):
			kept = append(kept, p)
		case r.Exists(p.from) && !linkHolds(r, p.heldLink):
			out = append(out, linkChange(r, p))
		}
	}
	clear(j.pending[len(kept):])
	j.pending = kept
	return out
}

func (j *LinkJournal) forget(l heldLink) {
	kept := j.pending[:0]
	for _, p := range j.pending {
		if p.heldLink != l {
			kept = append(kept, p)
		}
	}
	clear(j.pending[len(kept):])
	j.pending = kept
}

func (j *LinkJournal) waiting(l heldLink) bool {
	for _, p := range j.pending {
		if p.heldLink == l {
			return true
		}
	}
	return false
}

func hasLink(links []heldLink, l heldLink) bool {
	for _, have := range links {
		if have == l {
			return true
		}
	}
	return false
}

// linkHolds reports a link present again in the model.
func linkHolds(r inventory.Reader, l heldLink) bool {
	probe := inventory.Observation{Kind: inventory.Related,
		Entity: l.from, Relation: l.rel}
	_, selects := selectorKinds[l.from.Kind]
	if selects || (l.rel == inventory.Calls && usingKinds[l.from.Kind]) {
		probe = inventory.Observation{Kind: inventory.Observed,
			Entity: l.from}
	}
	return hasLink(linksOf(r, probe), l)
}

// linkChange is the change that records the end of p, attributed to
// whoever last edited the object that held it.
func linkChange(r inventory.Reader, p pendingLink) inventory.Observation {
	change := inventory.Change{
		At: p.at, Observed: p.at,
		Fields: []inventory.FieldChange{inventory.LinkRemoved(p.rel, p.to)},
	}
	for _, edit := range r.Changes(p.from, p.at.Add(-LinkFlapWindow)) {
		if _, _, isLink := edit.RemovedLink(); isLink || edit.Created ||
			edit.Deleted || edit.At.After(p.at.Add(LinkFlapWindow)) {
			continue
		}
		change.Actor, change.App = edit.Actor, edit.App
	}
	return inventory.Observation{
		Kind: inventory.Changed, Source: ObservationSource, At: p.at,
		Entity: p.from, Change: change,
	}
}
