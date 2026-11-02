package kube

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// WakeContextTail is how long after a wake-up ended a new problem still
// mentions it: something that fails right after the hold ran out was
// held because of it.
const WakeContextTail = 15 * time.Minute

// startupWarnings are the Warning events of a pod that is starting up
// slowly: its probes failing, or no node found for it yet.
var startupWarnings = map[string]bool{
	"Unhealthy": true, "FailedScheduling": true,
}

// WakeReport is what a wake-up cost, counted once it is over.
type WakeReport struct {
	Wake Wake
	// Blips counts the workloads with a pod that had startup warnings.
	Blips int
	// Failing counts those whose pod is still not ready.
	Failing int
}

// ReportWake counts the workloads of the wake-up whose pods had startup
// warnings, and how many of those still have a pod that is not ready.
// Only the workloads the wake-up started from zero count: a pod created
// meanwhile by a workload that was already running, or by none, is not
// part of what the wake-up line reports.
func ReportWake(r inventory.Reader, w Wake) WakeReport {
	report := WakeReport{Wake: w}
	seen := map[inventory.EntityID]bool{}
	failing := map[inventory.EntityID]bool{}
	for _, id := range r.Entities(KindPod) {
		pod, ok := r.Entity(id)
		if !ok || !podStartedInWake(pod, w) || !hadStartupWarning(r, id, w) {
			continue
		}
		owner, ok := wakeWorkload(r, id, w)
		if !ok {
			continue
		}
		seen[owner] = true
		if ready, _ := pod.Attribute(AttrReady); !isTrue(ready.Value) {
			failing[owner] = true
		}
	}
	report.Blips, report.Failing = len(seen), len(failing)
	return report
}

// maxWakeOwnerHops bounds the owner chain wakeWorkload follows: pod,
// ReplicaSet, Deployment, and an operator's resource above it.
const maxWakeOwnerHops = 4

// wakeWorkload is the owner of pod, at any level of its owner chain,
// that the wake-up started from zero. The chain can go on past it: an
// operator's resource or an Argo Rollout above the Deployment or
// ReplicaSet the wake-up saw scale.
func wakeWorkload(
	r inventory.Reader, pod inventory.EntityID, w Wake,
) (inventory.EntityID, bool) {
	level := []inventory.EntityID{pod}
	for range maxWakeOwnerHops {
		var next []inventory.EntityID
		for _, id := range level {
			next = append(next,
				r.Related(id, inventory.OwnedBy, inventory.Outgoing)...)
		}
		for _, owner := range next {
			if slices.Contains(w.Workloads, owner) {
				return owner, true
			}
		}
		level = next
	}
	return inventory.EntityID{}, false
}

func podStartedInWake(pod inventory.Entity, w Wake) bool {
	created, ok := pod.Attribute(AttrCreated)
	return ok && w.Covers(created.Value.AsTime())
}

func hadStartupWarning(
	r inventory.Reader, pod inventory.EntityID, w Wake,
) bool {
	for _, note := range r.Notes(pod, w.Start.Add(-wakeSlack)) {
		if note.Warning && startupWarnings[note.Reason] {
			return true
		}
	}
	return false
}

// ContextWake is the wake-up a new problem should mention: one going on,
// or one that ended less than WakeContextTail ago.
func ContextWake(r inventory.Reader, now time.Time) (Wake, bool) {
	w, ok := LatestWake(r, now)
	if !ok || now.Sub(w.End()) > WakeContextTail {
		return Wake{}, false
	}
	return w, true
}

func isTrue(v inventory.Value) bool {
	b, _ := v.AsBool()
	return b
}
