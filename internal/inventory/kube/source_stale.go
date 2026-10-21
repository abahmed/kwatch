package kube

import (
	"sort"
	"time"
)

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
