package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// Recent warnings bounds: the objects whose events are read, and the
// folded warnings kept, newest first.
const (
	maxWarningObjects = 64
	maxWarnings       = 10
)

// warningLookback is how long before the incident opened a Warning
// event still counts: the first warnings often come before kwatch
// opens the incident.
const warningLookback = 30 * time.Minute

// warningKey folds warnings with the same reason and message.
type warningKey struct{ reason, message string }

// warningsOf folds the Warning events of p's objects (its root, merged
// roots, failing objects, impact, pods and their containers) by reason
// and message, newest first. cut counts the folded warnings left out.
func warningsOf(
	model inventory.Reader, p incident.Incident,
) ([]RecentWarning, int) {
	since := p.Opened.Add(-warningLookback)
	folded := map[warningKey]*RecentWarning{}
	objects := map[warningKey]map[inventory.EntityID]bool{}
	for _, id := range incidentObjects(model, p) {
		for _, note := range model.Notes(id, since) {
			if !note.Warning || afterResolve(p, note) {
				continue
			}
			key := warningKey{note.Reason, redact.Evidence(note.Message)}
			addWarning(folded, key, note, id)
			if objects[key] == nil {
				objects[key] = map[inventory.EntityID]bool{}
			}
			objects[key][id] = true
		}
	}
	out := make([]RecentWarning, 0, len(folded))
	for key, w := range folded {
		w.Objects = len(objects[key])
		out = append(out, *w)
	}
	slices.SortFunc(out, func(a, b RecentWarning) int {
		return cmp.Or(b.LastSeen.Compare(a.LastSeen),
			cmp.Compare(b.Count, a.Count), cmp.Compare(a.Reason, b.Reason),
			cmp.Compare(a.Message, b.Message))
	})
	if len(out) > maxWarnings {
		return out[:maxWarnings], len(out) - maxWarnings
	}
	return out, 0
}

// afterResolve reports a note that started after p resolved: it is
// about something else.
func afterResolve(p incident.Incident, note inventory.Note) bool {
	first := note.FirstSeen
	if first.IsZero() {
		first = note.At
	}
	return !p.Resolved.IsZero() && first.After(p.Resolved)
}

// addWarning adds note, reported about id, to its folded warning.
func addWarning(
	folded map[warningKey]*RecentWarning, key warningKey,
	note inventory.Note, id inventory.EntityID,
) {
	w := folded[key]
	if w == nil {
		w = &RecentWarning{Reason: key.reason, Message: key.message}
		folded[key] = w
	}
	w.Count += max(note.Count, 1)
	first := note.FirstSeen
	if first.IsZero() {
		first = note.At
	}
	if w.FirstSeen.IsZero() || first.Before(w.FirstSeen) {
		w.FirstSeen = first
	}
	if note.At.After(w.LastSeen) || w.Object == "" ||
		(note.At.Equal(w.LastSeen) && objectText(id) < w.Object) {
		w.LastSeen = note.At
		w.Object = objectText(id)
	}
}

// objectText names an object for people: "pod shop/api-1", "node n1".
func objectText(id inventory.EntityID) string {
	if id.Namespace == "" {
		return string(id.Kind) + " " + id.Name
	}
	return string(id.Kind) + " " + id.Namespace + "/" + id.Name
}

// incidentObjects are the objects p is about, by name and at most
// maxWarningObjects: its root, merged roots, failing objects and
// impact, then its pods and their containers.
func incidentObjects(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	ids := append([]inventory.EntityID{p.Root}, p.Merged...)
	ids = append(ids, memberIDs(p)...)
	ids = append(ids, p.Impact...)
	for _, pod := range podsOf(model, p) {
		ids = append(ids, pod)
		ids = append(ids, containerIDs(model, pod)...)
	}
	seen := map[inventory.EntityID]bool{}
	out := make([]inventory.EntityID, 0, len(ids))
	for _, id := range ids {
		if id.Name == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == maxWarningObjects {
			break
		}
	}
	return out
}
