package kube

import (
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// maxDeniedSkips bounds how many retry passes a resource type that keeps
// being refused for lack of permission is left alone. A Role rarely
// changes while kwatch runs, and every try is a forbidden LIST in the API
// audit log of a least-privilege install. With the default 30-minute
// safety timer the longest wait is four hours; a restart tries at once.
const maxDeniedSkips = 7

// maxLoggedRefused bounds the type names of the refusal summary line.
const maxLoggedRefused = 20

// denial counts, for one resource type, its consecutive permission
// refusals and the retry passes skipped since the last try.
type denial struct{ refusals, skipped int }

// noteRefusal remembers a refusal of gvr; only a permission refusal
// counts toward the backoff. The caller holds d.mu.
func (d *DynamicSource) noteRefusal(
	gvr schema.GroupVersionResource, reason string,
) {
	if reason != ReasonPermissionDenied {
		delete(d.denials, gvr)
		return
	}
	if d.denials == nil {
		d.denials = map[schema.GroupVersionResource]*denial{}
	}
	if d.denials[gvr] == nil {
		d.denials[gvr] = &denial{}
	}
	d.denials[gvr].refusals++
}

// denialDue reports whether a retry pass should try gvr again. The first
// retry always does; each further permission refusal doubles the passes
// skipped (1, 3, 7), up to maxDeniedSkips. The caller holds d.mu.
func (d *DynamicSource) denialDue(gvr schema.GroupVersionResource) bool {
	den := d.denials[gvr]
	if den == nil || den.refusals <= 1 {
		return true
	}
	if den.skipped >= min(1<<(den.refusals-1)-1, maxDeniedSkips) {
		den.skipped = 0
		return true
	}
	den.skipped++
	return false
}

// forgetSyncedDenials clears the backoff of the types whose watch has
// synced: they work now. The caller holds d.mu.
func (d *DynamicSource) forgetSyncedDenials() {
	for gvr := range d.denials {
		if w, running := d.running[gvr]; running && w.hasSynced() {
			delete(d.denials, gvr)
		}
	}
}

// refusedSummaryLocked names the refused resource types, sorted, and how
// many were refused for permission. The caller holds d.mu.
func (d *DynamicSource) refusedSummaryLocked() (types string, denied int) {
	names := make([]string, 0, len(d.refused))
	for gvr, f := range d.refused {
		names = append(names, resourceName(gvr))
		if f.reason == ReasonPermissionDenied {
			denied++
		}
	}
	sort.Strings(names)
	if len(names) > maxLoggedRefused {
		more := len(names) - maxLoggedRefused
		names = append(names[:maxLoggedRefused], "+"+strconv.Itoa(more))
	}
	return strings.Join(names, ","), denied
}

// refusedChanged reports whether the refused types differ from the list
// last logged. An empty list is never news.
func (d *DynamicSource) refusedChanged(types string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	changed := types != "" && types != d.loggedRefused
	d.loggedRefused = types
	return changed
}

// logRefused writes one line for all the refused resource types of a
// discovery pass, and again only when the list changes, instead of one
// line per type at every retry.
func (d *DynamicSource) logRefused(types string, denied int) {
	if d.refusedChanged(types) {
		klog.InfoS("resource types unavailable; retried at a later "+
			"rediscovery", "component", "inventory", "operation",
			"discover", "permissionDenied", denied, "types", types)
	}
}
