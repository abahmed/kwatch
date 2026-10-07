package kube

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// ScaleDownMin is how many workloads set to zero replicas together
	// make a planned scale-down: a cluster put to sleep, not one
	// workload someone switched off.
	ScaleDownMin = 5
	// ScaleDownSpan is how close in time two workloads' scale-downs are
	// for them to belong together.
	ScaleDownSpan = 15 * time.Minute
)

// ScaledDownInBatch reports whether workload was set to zero replicas at
// at together with many others: a planned scale-down. Workloads that
// went to zero in it are at zero on purpose; their routes and endpoints
// are left as they were.
func ScaledDownInBatch(
	r inventory.Reader, workload inventory.EntityID, at time.Time,
) bool {
	if at.IsZero() {
		return false
	}
	together := 0
	for _, kind := range []inventory.Kind{KindDeployment, KindStatefulSet} {
		for _, id := range r.Entities(kind) {
			if id == workload || scaledToZeroNear(r, id, at) {
				together++
			}
		}
	}
	return together >= ScaleDownMin
}

// scaledToZeroNear reports a workload whose replicas were set to zero
// within ScaleDownSpan of at.
func scaledToZeroNear(
	r inventory.Reader, id inventory.EntityID, at time.Time,
) bool {
	for _, change := range r.Changes(id, at.Add(-ScaleDownSpan)) {
		if change.At.After(at.Add(ScaleDownSpan)) {
			continue
		}
		for _, f := range change.Fields {
			if after, err := strconv.Atoi(f.After); f.Path ==
				"spec.replicas" && err == nil && after == 0 &&
				f.Before != "0" {
				return true
			}
		}
	}
	return false
}
