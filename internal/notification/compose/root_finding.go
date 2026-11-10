package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// rootFinding is the root's own most severe condition, if it has one,
// or the one its cause names.
// Symptoms (a workload below its replicas) never lead the message.
func rootFinding(
	p incident.Incident, members []detection.Finding,
) *detection.Finding {
	var first *detection.Finding
	for i := range members {
		m := &members[i]
		if m.Entity != p.Root || m.Symptom || m.Advisory {
			continue
		}
		if causeModeOf(p, *m) {
			return m
		}
		if first == nil {
			first = m
		}
	}
	return first
}

// causeModeOf reports a finding of the root that is the failure its
// cause names: of a node with several failures of its own (CPU pressure
// and no pod IPs left), the lead is the one that explains the incident,
// not the most severe or the oldest.
func causeModeOf(p incident.Incident, m detection.Finding) bool {
	cause := p.Cause
	return cause != nil && cause.Root == p.Root && cause.Mode != "" &&
		modeOf(m) == cause.Mode
}
