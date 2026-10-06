package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// withChanges gives a message about an incident without a cause the
// latest changes next to it, so it can say "bob changed configmap
// app-config at 21:10" instead of only "nothing explains this". The
// writer reads the changes as data, never the model.
func (a *announcer) withChanges(
	d incident.Decision, at time.Time,
) incident.Decision {
	if d.Incident.Cause != nil || a.history == nil ||
		(d.Action != incident.Announce && d.Action != incident.Update) {
		return d
	}
	d.Facts.Changes = incident.RecentChanges(a.history, d.Incident.Root, at)
	return d
}
