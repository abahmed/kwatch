package compose

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// firstRolloutLead says plainly that a workload has never worked: "api
// in shop has never become healthy since it was created 12 minutes
// ago." It applies only when the workload's own finding says its first
// rollout is still unhealthy, and nothing else is blamed: a named cause
// is the better lead.
func firstRolloutLead(f caseFacts) (string, bool) {
	root := f.p.Root
	if !f.ok || !incident.IsWorkload(root.Kind) || root != leadSubject(f) ||
		blamesAnother(f) {
		return "", false
	}
	created, ok := neverHealthySince(f, root)
	if !ok {
		return "", false
	}
	return f.leadName(root) + " has never become healthy since " +
		"it was created " + humanDuration(f.now.Sub(created)) + " ago", true
}

// neverHealthySince is when the workload was created, from the member
// finding of the workload that says its first rollout never worked.
func neverHealthySince(
	f caseFacts, workload inventory.EntityID,
) (time.Time, bool) {
	for _, m := range f.members {
		if m.Entity != workload {
			continue
		}
		created, err := time.Parse(time.RFC3339,
			evidence(m, detection.EvidenceNeverHealthy))
		if err == nil {
			return created, true
		}
	}
	return time.Time{}, false
}

// blamesAnother reports a cause that is neither the incident's root
// nor "it fails on its own".
func blamesAnother(f caseFacts) bool {
	cause := f.p.Cause
	return cause != nil && !selfCause(cause) && cause.Root != f.p.Root
}
