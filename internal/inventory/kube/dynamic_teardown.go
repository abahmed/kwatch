package kube

import "k8s.io/apimachinery/pkg/runtime/schema"

// Dropping and stopping the informers a DynamicSource no longer wants.

// dropUnwanted stops the types the plan no longer wants and returns the
// watches whose entities are gone. A type the budget dropped is parked
// quietly: its objects still exist. A type no longer discovered is
// removed, but only after a complete discovery. The caller holds d.mu.
func (d *DynamicSource) dropUnwanted(
	desired, skipped []plannedResource, complete bool,
) []*dynamicWatch {
	wanted := gvrSet(desired)
	overBudget := gvrSet(skipped)
	var retired []*dynamicWatch
	for gvr, w := range d.running {
		switch {
		case wanted[gvr]:
		case overBudget[gvr]:
			w.cancel()
			delete(d.running, gvr)
			d.parked[gvr] = w
		case complete:
			w.cancel()
			delete(d.running, gvr)
			retired = append(retired, w)
		}
	}
	for gvr, w := range d.parked {
		if complete && !overBudget[gvr] && !wanted[gvr] {
			delete(d.parked, gvr)
			retired = append(retired, w)
		}
	}
	for gvr, f := range d.refused {
		if !complete || wanted[gvr] || overBudget[gvr] {
			continue
		}
		delete(d.refused, gvr)
		if f.kept != nil {
			retired = append(retired, f.kept)
		}
	}
	return retired
}

func gvrSet(
	resources []plannedResource,
) map[schema.GroupVersionResource]bool {
	out := make(map[schema.GroupVersionResource]bool, len(resources))
	for _, r := range resources {
		out[r.gvr] = true
	}
	return out
}

// stopAll cancels every informer on shutdown. Entities are not reported
// gone: the whole source is stopping, not the resource types.
func (d *DynamicSource) stopAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for gvr, w := range d.running {
		w.cancel()
		w.admission.releaseAll()
		delete(d.running, gvr)
	}
	for gvr, w := range d.parked {
		w.admission.releaseAll()
		delete(d.parked, gvr)
	}
	for gvr, f := range d.refused {
		if f.kept != nil {
			f.kept.admission.releaseAll()
		}
		delete(d.refused, gvr)
	}
}
