package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
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
		return d
	}
	d.Facts.Changes = incident.NearChanges(a.history, d.Incident.Root, at,
		d.Incident.Onset(), d.Incident.Cause.Change)
	return d
}
