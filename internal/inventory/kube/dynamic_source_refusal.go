package kube

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
)

// Dynamic kind reason codes, in addition to the Reason* codes of
// source_sync.go.
const (
	// ReasonWatchBudget marks a type left out by the watch budget.
	ReasonWatchBudget = "watch_budget_exceeded"
	// ReasonObjectCap marks a type with objects left out by a cap.
	ReasonObjectCap = "object_cap_reached"
)

// KindState is what the dynamic source knows of one entity kind, over
// every resource type of that kind.
type KindState struct {
	// Synced is true when every type of the kind has listed and reports
	// every object, so an object missing from the model does not exist.
	Synced bool
	// Reason is a bounded code for why the kind is not synced.
	Reason string
}

// KindState reports the state of kind and whether the dynamic source
// watches, or tried to watch, any resource type of it.
func (d *DynamicSource) KindState(kind inventory.Kind) (KindState, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	found, reason := false, ""
	note := func(r string) {
		found = true
		if reason == "" {
			reason = r
		}
	}
	for _, w := range d.running {
		if w.kind != kind {
			continue
		}
		switch {
		case !w.hasSynced() || w.takingOver():
			note(ReasonSyncPending)
		case w.admission.capped():
			note(ReasonObjectCap)
		default:
			found = true
		}
	}
	for _, f := range d.refused {
		if f.kind == kind {
			note(f.reason)
		}
	}
	for _, issue := range d.left {
		if issue.kind == kind {
			note(issue.reason)
		}
	}
	return KindState{Synced: found && reason == "", Reason: reason}, found
}

// refuse stops a watch the API refused, so its reflector does not retry
// until the safety timer re-discovers it. Only a NotFound refusal means
// the type is gone and reports its entities gone. A permission refusal
// keeps them: the objects most likely still exist, and the kind stays
// not verifiable until a restarted watch has synced. An error from a
// watch that is no longer the running one (refused, retired or replaced
// already) is ignored.
func (d *DynamicSource) refuse(
	gvr schema.GroupVersionResource, w *dynamicWatch, err error,
) {
	reason := syncFailureReason(err)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running[gvr] != w {
		return
	}
	klog.InfoS("resource type unavailable; retried at next rediscovery",
		"component", "inventory", "operation", "watch",
		"resource", gvr.String(), "reason", reason)
	registry := metrics.DefaultRegistry()
	registry.OptionalAPIUnavailable.Add(1)
	if !w.hasSynced() {
		registry.WatcherSyncFailures.Add(1)
	}
	w.cancel()
	delete(d.running, gvr)
	f := &refusal{kindIssue: kindIssue{w.kind, reason}}
	d.refused[gvr] = f
	if !apierrors.IsNotFound(err) {
		f.kept = w
		return
	}
	// The caller is the watch's own reflector, which the informer
	// goroutine (counted in wg) runs, so the retire cannot wait here.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.retire(w.parent, w)
	}()
}

// retire waits for a cancelled informer to stop, so no late notification
// can follow, then reports every object it admitted as gone: the
// resource type is no longer served, and entities left in the model
// would never be updated or resolved again.
func (d *DynamicSource) retire(ctx context.Context, w *dynamicWatch) {
	if !stopped(ctx, w.done) {
		return
	}
	d.reportGone(ctx, w, w.admission.dropMissing(
		func(string) bool { return false }))
}

// takeOver waits for a restarted watch to sync, then reports gone the
// objects the previous watch admitted that no longer exist.
func (d *DynamicSource) takeOver(ctx context.Context, w *dynamicWatch) {
	defer close(w.handedOver)
	if !cache.WaitForCacheSync(ctx.Done(), w.hasSynced) {
		return
	}
	d.reportGone(ctx, w, w.admission.dropMissing(func(key string) bool {
		_, exists, err := w.store.GetByKey(key)
		return exists || err != nil
	}))
}

func (d *DynamicSource) reportGone(
	ctx context.Context, w *dynamicWatch, objects []any,
) {
	now := d.cfg.Now()
	var observations []inventory.Observation
	for _, obj := range objects {
		observations = append(observations,
			w.translator.Deleted(obj, now)...)
	}
	if len(observations) > 0 {
		d.cfg.Submit(ctx, observations...)
	}
}
