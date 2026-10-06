package delivery

import "k8s.io/klog/v2"

// An issue tracker keeps one issue per incident key. A resolve for a key
// it has no issue for (kwatch restarted and the saved map lacks it, or
// the issue was never created) has nothing to close, and the provider
// can only skip it. The skip must not be silent: the incident may have
// an issue that nobody closes.

// resolveWithoutMapping reports a resolve sent to a tracker that keeps
// thread state and has none for the job's key.
func resolveWithoutMapping(entry *providerEntry, job deliverJob) bool {
	if !job.isResolve() || !skipsPlainMessages(entry.provider) {
		return false
	}
	if lookup, ok := entry.provider.(ThreadLookup); ok {
		return !lookup.HasThread(job.key())
	}
	tracker, ok := entry.provider.(ThreadStateProvider)
	if !ok {
		return false
	}
	_, mapped := tracker.SnapshotThreads()[job.key()]
	return !mapped
}

// warnUnmappedResolve logs a resolve that has no issue to close.
func warnUnmappedResolve(entry *providerEntry, job deliverJob) {
	if resolveWithoutMapping(entry, job) {
		klog.Warningf("delivery: resolve for %q (alert key %q) has no "+
			"issue mapped at %s; nothing is closed at the tracker",
			job.key(), job.incident.DedupKey, entry.provider.Name())
	}
}
