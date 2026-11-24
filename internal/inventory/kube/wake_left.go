package kube

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// LeftBehindGrace is how long after the scale-up that undid a
	// planned scale-down was over a workload of the scale-down may
	// still be at zero replicas: past it, it was left behind.
	LeftBehindGrace = 15 * time.Minute
	// leftBehindRecheck is how often a workload a planned scale-down
	// set to zero is looked at again, while its peers may be coming
	// back: their starts do not touch it.
	leftBehindRecheck = 5 * time.Minute
)

// LeftBehind reports a workload set to zero replicas at at by a planned
// scale-down (ScaledDownInBatch) that the scale-up undoing it left at
// zero: at least ScaleDownMin of the workloads scaled down with it were
// started from zero again since, and LeftBehindGrace has passed since
// the wake-up of their latest start was over. Until then recheck is how
// long to wait before asking again; zero when it is decided.
func LeftBehind(
	r inventory.Reader, workload inventory.EntityID, at, now time.Time,
) (left bool, recheck time.Duration) {
	back, latest := 0, time.Time{}
	for _, kind := range []inventory.Kind{KindDeployment, KindStatefulSet} {
		for _, id := range r.Entities(kind) {
			if id == workload || !scaledToZeroNear(r, id, at) {
				continue
			}
			if started, ok := startedFromZero(r, id, at); ok {
				back++
				latest = maxTime(latest, started)
			}
		}
	}
	if back < ScaleDownMin {
		return false, leftBehindRecheck
	}
	due := latest.Add(WakeQuiet + LeftBehindGrace)
	if now.Before(due) {
		return false, due.Sub(now)
	}
	return true, 0
}

// ZeroedAt is when the workload's replicas were last set from some to
// zero since since, per its recorded changes.
func ZeroedAt(
	r inventory.Reader, id inventory.EntityID, since time.Time,
) (time.Time, bool) {
	var at time.Time
	for _, change := range r.Changes(id, since) {
		for _, f := range change.Fields {
			after, err := strconv.Atoi(f.After)
			if f.Path == "spec.replicas" && err == nil && after == 0 &&
				f.Before != "0" && change.At.After(at) {
				at = change.At
			}
		}
	}
	return at, !at.IsZero()
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
