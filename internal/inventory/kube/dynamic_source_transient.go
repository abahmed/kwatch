package kube

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	// refusalGrace is how long a watch that already synced keeps
	// retrying through refusals before it is refused. A CRD that is
	// deleted and recreated, or an API server that restarts, refuses for
	// a short while and then serves again; the reflector's own backoff
	// makes about eight attempts in this time.
	refusalGrace = 90 * time.Second
	// refusalGap is the quiet time after which a new refusal starts a
	// new grace period: the watch recovered in between.
	refusalGap = time.Minute
)

// watchError returns the error handler of one watch. A refusal ends the
// watch (see refuse) unless the watch had synced and the refusal is
// still within its grace period. The handler names its own watch: a
// reflector that is still stopping after a restart must not refuse its
// replacement.
func (d *DynamicSource) watchError(
	gvr schema.GroupVersionResource, w *dynamicWatch,
) cache.WatchErrorHandlerWithContext {
	return func(ctx context.Context, r *cache.Reflector, err error) {
		if !refusedByAPI(err) {
			d.noteTransient(gvr, w)
			cache.DefaultWatchErrorHandler(ctx, r, err)
			return
		}
		if d.retryingRefusal(gvr, w) {
			klog.V(2).InfoS("resource type refused; retrying",
				"component", "inventory", "operation", "watch",
				"resource", gvr.String(), "error", err)
			return
		}
		d.refuse(gvr, w, err)
	}
}

// retryingRefusal notes a refusal of a running watch and reports whether
// the reflector should keep retrying. Only a watch that had synced
// retries: a type refused from the start is a real gap, such as missing
// permission, and keeps its refusal.
func (d *DynamicSource) retryingRefusal(
	gvr schema.GroupVersionResource, w *dynamicWatch,
) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running[gvr] != w || !w.hasSynced() {
		return false
	}
	now := d.cfg.Now()
	if w.lastRefusal.IsZero() || now.Sub(w.lastRefusal) > refusalGap {
		w.firstRefusal = now
	}
	w.lastRefusal = now
	return now.Sub(w.firstRefusal) < refusalGrace
}
