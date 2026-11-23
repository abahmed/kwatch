package pipeline

import "github.com/abahmed/kwatch/internal/incident"

// withShape adds what the structured evidence says about the shape of
// an incident: what ties its members together, what its failing
// replicas share that the healthy ones do not, and whether users reach
// it. The writer reads them as data; it never reads the model.
func (a *announcer) withShape(d incident.Decision) incident.Decision {
	if d.Action == incident.Resolve || a.history == nil {
		return d
	}
	d.Facts.Grouping = incident.GroupingOf(a.history, d.Incident)
	d.Facts.Split = incident.ReplicaSplitOf(a.history, d.Incident)
	d.Facts.Users = incident.UsersOf(a.history, d.Incident)
	return a.withReplicas(d)
}

// withReplicas adds how many replicas of the root workload are ready, so
// a title in a roll-up, a summary or a digest can say the scale of the
// problem as the model reads it now.
func (a *announcer) withReplicas(d incident.Decision) incident.Decision {
	if d.Action == incident.Resolve || a.history == nil {
		return d
	}
	d.Facts.Replicas = incident.ReplicaCountOf(a.history, d.Incident)
	return d
}
