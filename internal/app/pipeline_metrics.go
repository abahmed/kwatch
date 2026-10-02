package app

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
)

// countDecision counts one lifecycle decision. Startup summaries carry no
// action and are not counted. It reports whether d was counted.
func countDecision(reg *metrics.Registry, d incident.Decision) bool {
	action := decisionAction(d.Action)
	if action == "" {
		return false
	}
	reg.IncIncident(action)
	return true
}

// refreshOpenIncidents sets the open-incident gauge. It copies every
// incident, so it runs off the decision loop.
func refreshOpenIncidents(
	reg *metrics.Registry, incidents func() []incident.Incident,
) {
	reg.IncidentsOpen.Store(int64(countOpen(incidents())))
}

func decisionAction(a incident.Action) string {
	switch a {
	case incident.Announce:
		return "announce"
	case incident.Update:
		return "update"
	case incident.Resolve:
		return "resolve"
	}
	return ""
}

func countOpen(all []incident.Incident) int {
	open := 0
	for i := range all {
		if all[i].State != incident.Resolved {
			open++
		}
	}
	return open
}
