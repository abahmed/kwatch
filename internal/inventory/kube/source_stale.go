package kube

import (
	"sort"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// progressOf is the mark a watch of resource compares to tell that a
// list or watch got through after a failure: the informer's resource
// version and, with SourceConfig.Answered, the number of lists and
// watches the API server answered. The count matters for kinds that
// rarely change (storage classes, webhook configurations): a relist
// after a blip returns the version the informer already had, so the
// version alone would leave the kind stale until one of its objects
// changed.
func (s *Source) progressOf(
	resource Resource, informer interface{ LastSyncResourceVersion() string },
) func() string {
	answered := s.cfg.Answered
	if answered == nil {
		return informer.LastSyncResourceVersion
	}
	return func() string {
		return informer.LastSyncResourceVersion() + "/" +
			strconv.FormatUint(answered(resource.Group, resource.Name), 10)
	}
}

// noteFailure starts, or restarts, the run of failures of a synced watch.
// A run that began at an older resource version ended when a list or watch
// got through (the version moved). The caller holds w.mu.
func (w *watchState) noteFailure() {
	version := w.lastVersion()
	if w.failingSince.IsZero() || version != w.failVersion {
		w.failingSince, w.failVersion = w.now(), version
	}
}

// stale reports whether a synced watch has failed without a single list or
// watch getting through for at least bound: the cache holds what was true
// when the watch broke, and nothing says it still is. A moved resource
// version means the watch works again.
func (w *watchState) stale(now time.Time, bound time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failingSince.IsZero() {
		return false
	}
	if w.lastVersion() != w.failVersion {
		w.failingSince = time.Time{}
		return false
	}
	return now.Sub(w.failingSince) >= bound
}

// Stale lists the synced resources whose watch keeps failing: no list or
// watch got through for OptionalSyncTimeout. They are still synced and
// readiness does not change; health shows them as degraded so a cluster
// that stopped answering is not mistaken for a quiet one. A resource
// whose access was refused is listed by Unavailable instead.
func (s *Source) Stale() []SourceStatus {
	now := s.cfg.Now()
	var out []SourceStatus
	for _, w := range s.watches {
		if w.hasSynced() && w.stale(now, s.cfg.OptionalSyncTimeout) {
			out = append(out, SourceStatus{
				Resource: w.resource.Name, Group: w.resource.Group,
				Required: w.required, Reason: ReasonSyncFailed,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Resource < out[j].Resource
	})
	return out
}

// Fresh reports whether kind can be observed now and its watch still
// gets through: it is verifiable and not stale (see Stale). The detection
// registry reads it (Registry.SetFresh) so a detector that would raise
// from the passage of time alone stays undecided over a cache that
// stopped moving. Kinds no watch lists are fresh. A dynamic kind is
// fresh unless a synced watch of it is stale (KindState.Stale): whether
// it is verifiable is for Synced and Verifiable to say.
func (s *Source) Fresh(kind inventory.Kind) bool {
	listed := kind
	if listed == KindContainer {
		listed = KindPod
	}
	resource, typed := resourceOfKind()[listed]
	if !typed {
		state, ok := s.dynamicState(kind)
		return !ok || !state.Stale
	}
	if !s.Verifiable(kind) {
		return false
	}
	now := s.cfg.Now()
	for _, w := range s.watches {
		if w.resource == resource && w.hasSynced() {
			return !w.stale(now, s.cfg.OptionalSyncTimeout)
		}
	}
	return true
}
