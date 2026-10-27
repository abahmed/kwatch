package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// crashLogMarkers are the lower-case words of a crash log line that
// says plainly why the program stopped: a fatal error or a connection
// that did not arrive.
var crashLogMarkers = []string{"fatal", "panic:", "connection refused",
	"dial tcp", "i/o timeout", "no route to host",
	"connection timed out", "network is unreachable", "no such host"}

// withoutOwnPredecessor drops the deletion of the affected workload when
// the same workload was created again afterwards: "kubectl delete" then
// "kubectl create" leaves a new workload with no earlier working state,
// so the old one's deletion did not break it.
func withoutOwnPredecessor(
	f caseFacts, changes []inventory.Change,
) []inventory.Change {
	workload := affectedWorkload(f)
	if !incident.IsWorkload(workload.Kind) {
		return changes
	}
	out := make([]inventory.Change, 0, len(changes))
	for _, change := range changes {
		if change.Deleted && change.Entity == workload &&
			recreatedAfter(changes, change) {
			continue
		}
		out = append(out, change)
	}
	return out
}

// recreatedAfter reports a creation of the deleted object at or after
// its deletion.
func recreatedAfter(
	changes []inventory.Change, deleted inventory.Change,
) bool {
	for _, change := range changes {
		if change.Created && change.Entity == deleted.Entity &&
			!change.At.Before(deleted.At) {
			return true
		}
	}
	return false
}

// neverWorked reports a workload that has failed since it was created:
// its own finding says so, or its creation is among the changes.
func neverWorked(f caseFacts) bool {
	workload := affectedWorkload(f)
	if !incident.IsWorkload(workload.Kind) {
		return false
	}
	if _, ok := neverHealthySince(f, workload); ok {
		return true
	}
	if change := blamedChange(f); change != nil &&
		ownCreation(f, *change) {
		return true
	}
	for _, change := range f.changes {
		if change.Created && change.Entity == workload {
			return true
		}
	}
	return false
}

// crashLogExplains reports a crash log line that already says why the
// program stopped.
func crashLogExplains(f caseFacts) bool {
	texts := []string{investigated(f, incident.FactError)}
	for _, m := range f.members {
		texts = append(texts, evidence(m, detection.EvidenceError))
	}
	for _, text := range texts {
		lower := strings.ToLower(text)
		for _, marker := range crashLogMarkers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}
