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

// handOverVersions parks the watch of a type whose served version
// changed (a CRD's preferred version moving from v1beta1 to v1) under
// the new version, so the new watch takes its objects over (takeOver)
// instead of the old one being retired and its live objects reported
// gone: entity IDs carry no version. The caller holds d.mu.
func (d *DynamicSource) handOverVersions(desired []plannedResource) {
	wanted := gvrSet(desired)
	for _, r := range desired {
		_, running := d.running[r.gvr]
		_, parked := d.parked[r.gvr]
		if running || parked {
			continue
		}
		if w, ok := d.takeOldVersion(wanted, r.gvr); ok {
			d.parked[r.gvr] = w
		}
	}
}

// takeOldVersion removes and returns the running (then stopped) or
// parked watch of another, no longer wanted version of target's
// resource. The caller holds d.mu.
func (d *DynamicSource) takeOldVersion(
	wanted map[schema.GroupVersionResource]bool,
	target schema.GroupVersionResource,
) (*dynamicWatch, bool) {
	old := func(gvr schema.GroupVersionResource) bool {
		return gvr != target && !wanted[gvr] && sameResource(gvr, target)
	}
	for gvr, w := range d.running {
		if old(gvr) {
			w.cancel()
			delete(d.running, gvr)
			return w, true
		}
	}
	for gvr, w := range d.parked {
		if old(gvr) {
			delete(d.parked, gvr)
			return w, true
		}
	}
	return nil, false
}

// sameResource reports two versions of one group and resource.
func sameResource(a, b schema.GroupVersionResource) bool {
	return a.Group == b.Group && a.Resource == b.Resource
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
