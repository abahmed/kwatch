package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// withChanges gives a message the changes next to the failure, with what
// each one edited. Without a cause it says "bob changed configmap
// app-config at 21:10" instead of only "nothing explains this". With a
// cause it names the other nearby changes made before the failure began,
// ranked by how close they sit to the root. The writer reads the changes
// as data, never the model.
func (a *announcer) withChanges(
	d incident.Decision, at time.Time,
) incident.Decision {
	if a.history == nil ||
		(d.Action != incident.Announce && d.Action != incident.Update) {
		return d
	}
	if d.Incident.Cause == nil {
		d.Facts.Changes = incident.RecentChanges(
			a.history, d.Incident.Root, at)
	} else {
		d.Facts.Changes = incident.NearChanges(a.history, d.Incident.Root,
			at, d.Incident.Onset(), d.Incident.Cause.Change)
	}
	return withWindow(d, a.history, at)
}

// withWindow adds the health window, when there is one, and takes its
// changes out of the list of recent changes: a message says each once.
func withWindow(
	d incident.Decision, history inventory.HistoryReader, at time.Time,
) incident.Decision {
	d.Facts.Window = incident.WindowOf(history, d.Incident, at)
	if d.Facts.Window == nil {
		return d
	}
	var rest []inventory.Change
	for _, change := range d.Facts.Changes {
		if !inWindow(d.Facts.Window, change) {
			rest = append(rest, change)
		}
	}
	d.Facts.Changes = rest
	return d
}

func inWindow(w *incident.HealthWindow, change inventory.Change) bool {
	for _, named := range w.Changes {
		if named.Entity == change.Entity && named.At.Equal(change.At) {
			return true
		}
	}
	return false
}
