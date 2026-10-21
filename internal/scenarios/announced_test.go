package scenarios

import (
	"slices"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// The accuracy gate judges each incident in its final state, so a wrong
// root that a later message corrects goes unseen. These checks judge the
// root at every message instead: what people were told, when they were
// told it.

// announcement is what one message said about an incident's root: an
// announcement, an update, or an incident listed in a roll-up, a startup
// summary or a digest.
type announcement struct {
	at       time.Time
	incident incident.Incident
}

func (a announcement) root() string { return a.incident.Root.String() }

func (a announcement) cause() *rootcause.CauseRecord { return a.incident.Cause }

// announcements lists every message that stated an incident's root, in
// the order people heard them. A resolve states no root, and a decision
// nobody received (dropped) was not heard.
func announcements(result replay.Result) []announcement {
	var out []announcement
	add := func(d incident.Decision, at time.Time) {
		if d.Incident.ID != "" &&
			(d.Action == incident.Announce || d.Action == incident.Update) {
			out = append(out, announcement{at: at, incident: d.Incident})
		}
	}
	for i, d := range result.Decisions {
		add(d, result.Times[i])
	}
	for _, c := range result.Carried {
		if c.Carrier != announce.CarrierDropped {
			add(c.Decision, c.At)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].at.Before(out[j].at)
	})
	return out
}

// allowedRoots are the entities a label lets a message name as a cause:
// its roots, and the roots of the real incidents it calls unrelated.
func allowedRoots(e expectation) map[string]bool {
	allowed := map[string]bool{}
	for _, root := range slices.Concat(e.roots(), e.Unrelated) {
		allowed[root] = true
	}
	return allowed
}

// ownership maps an entity to the entities that own it or contain it, as
// the replay log related them: a container to its pod, a pod to its
// ReplicaSet or DaemonSet, a ReplicaSet to its Deployment.
type ownership map[string][]string

// ownershipOf reads the ownership relations of a log.
func ownershipOf(log replay.Log) ownership {
	owners := ownership{}
	for _, entry := range log.Entries {
		o := entry.Observation
		if o.Kind != inventory.Related ||
			(o.Relation != inventory.OwnedBy &&
				o.Relation != inventory.PartOf) {
			continue
		}
		for _, target := range o.Targets {
			child := o.Entity.String()
			if !slices.Contains(owners[child], target.String()) {
				owners[child] = append(owners[child], target.String())
			}
		}
	}
	return owners
}

// allows reports whether id, or an entity that owns it, is allowed: a
// message that names the pod of a labelled DaemonSet names the right
// thing at a finer grain, which is not a wrong root.
func (o ownership) allows(id string, allowed map[string]bool) bool {
	seen := map[string]bool{}
	queue := []string{id}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if allowed[next] {
			return true
		}
		if !seen[next] {
			seen[next] = true
			queue = append(queue, o[next]...)
		}
	}
	return false
}

// wrongRoots lists the entities a message stated as the cause that the
// label does not allow: the incident's root, the cause's root and the
// rival of a hedge. An incident with no stated cause says the cause is
// unknown, which is never wrong, and an ambiguous label tolerates a
// hedge between the root and its rival.
func wrongRoots(
	e expectation, allowed map[string]bool, owners ownership,
	a announcement,
) []string {
	cause := a.cause()
	if cause == nil || slices.Contains(e.Unrelated, a.root()) ||
		e.Ambiguous && cause.Rival != nil {
		return nil
	}
	stated := []string{a.root(), cause.Root.String()}
	if cause.Rival != nil {
		stated = append(stated, cause.Rival.Root.String())
	}
	var wrong []string
	for _, id := range stated {
		if !owners.allows(id, allowed) && !slices.Contains(wrong, id) {
			wrong = append(wrong, id)
		}
	}
	return wrong
}

// wrongRootsAnnounced judges every message. first is the wrong roots of
// the first messages people heard, all those at the earliest moment
// (a roll-up lists several together); overall is the wrong roots over every
// message. Messages about the label's unrelated incidents are left out.
func wrongRootsAnnounced(
	e expectation, owners ownership, result replay.Result,
) (first, overall []string) {
	allowed := allowedRoots(e)
	var firstAt time.Time
	for _, a := range announcements(result) {
		if slices.Contains(e.Unrelated, a.root()) {
			continue
		}
		if firstAt.IsZero() {
			firstAt = a.at
		}
		for _, id := range wrongRoots(e, allowed, owners, a) {
			if a.at.Equal(firstAt) && !slices.Contains(first, id) {
				first = append(first, id)
			}
			if !slices.Contains(overall, id) {
				overall = append(overall, id)
			}
		}
	}
	return first, overall
}

// wrongFirstRoot reports a scenario whose first message stated a wrong
// root and which has not been labelled as a legitimate revision.
func (v verdict) wrongFirstRoot() bool {
	return !v.expect.RevisesRoot && len(v.firstWrong) > 0
}
