package kube

import (
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// dynamicStaleAfter is how long a dynamic watch may fail without a single
// list or watch getting through before its kind counts as failed (not
// synced yet) or stale (synced). It is the bound typed watches use.
const dynamicStaleAfter = DefaultOptionalSyncTimeout

// dynamicFailure is the run of transient failures (server errors,
// timeouts) of one running dynamic watch: the dynamic side of
// watchState.noteFailure. DynamicSource.mu guards it.
type dynamicFailure struct {
	failingSince time.Time
	failVersion  string
	// progress is the mark that moves when a list or watch got through
	// (see DynamicSource.progressOf); nil reads as no progress.
	progress func() string
}

func (f *dynamicFailure) version() string {
	if f.progress == nil {
		return ""
	}
	return f.progress()
}

// stale reports whether the watch has failed without progress for at
// least bound. Progress since the run began ends the run.
func (f *dynamicFailure) stale(now time.Time, bound time.Duration) bool {
	if f.failingSince.IsZero() {
		return false
	}
	if f.version() != f.failVersion {
		f.failingSince = time.Time{}
		return false
	}
	return now.Sub(f.failingSince) >= bound
}

// progressOf mirrors Source.progressOf for a dynamic informer: its
// resource version and, with DynamicConfig.Answered, the number of lists
// and watches the API server answered for the type's group and resource.
func (d *DynamicSource) progressOf(
	gvr schema.GroupVersionResource,
	informer interface{ LastSyncResourceVersion() string },
) func() string {
	answered := d.cfg.Answered
	if answered == nil {
		return informer.LastSyncResourceVersion
	}
	return func() string {
		return informer.LastSyncResourceVersion() + "/" +
			strconv.FormatUint(answered(gvr.Group, gvr.Resource), 10)
	}
}

// noteTransient starts, or restarts, the failure run of the running watch
// w after an error a retry may fix. A run that began at an older mark
// ended when a list or watch got through. An error of a watch that is no
// longer the running one is ignored.
func (d *DynamicSource) noteTransient(
	gvr schema.GroupVersionResource, w *dynamicWatch,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running[gvr] != w {
		return
	}
	f := &w.failure
	version := f.version()
	if f.failingSince.IsZero() || version != f.failVersion {
		f.failingSince, f.failVersion = d.failureNow(), version
	}
}

// failureNow is the time failure runs are measured in. A source built
// without a clock (some tests) measures none: every run stays fresh.
func (d *DynamicSource) failureNow() time.Time {
	if d.cfg.Now == nil {
		return time.Time{}
	}
	return d.cfg.Now()
}

// staleLocked reports whether the watch kept failing past
// dynamicStaleAfter. The caller holds d.mu.
func (d *DynamicSource) staleLocked(w *dynamicWatch, now time.Time) bool {
	return w.failure.stale(now, dynamicStaleAfter)
}

// takingOver reports whether the watch still hands over from its
// predecessor.
func (w *dynamicWatch) takingOver() bool {
	if w.handedOver == nil {
		return false
	}
	select {
	case <-w.handedOver:
		return false
	default:
		return true
	}
}
