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

// createdFailureWindow is how soon after its creation a workload must
// have started failing for the message to say it never worked.
const createdFailureWindow = 3 * time.Minute

// createdLead says that a workload which failed from the start never
// worked: the blamed change is its own creation, and nothing is read into
// "its dependents failed after it changed". It covers Deployments,
// StatefulSets and DaemonSets, whether their pods are unready or crash.
func createdLead(f caseFacts) (string, bool) {
	change := blamedChange(f)
	if change == nil || !ownCreation(f, *change) {
		return "", false
	}
	onset := f.p.Onset()
	if !onset.IsZero() && onset.Sub(change.At) > createdFailureWindow {
		return "", false
	}
	workload := affectedWorkload(f)
	return f.leadName(workload) + " has never worked since it was " +
		"created at " + clock(change.At), true
}

// ownCreation reports that change is the creation of the workload the
// message is about.
func ownCreation(f caseFacts, change inventory.Change) bool {
	workload := affectedWorkload(f)
	return change.Created && change.Entity == workload &&
		incident.IsWorkload(workload.Kind)
}
