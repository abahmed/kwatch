package delivery

import "k8s.io/klog/v2"

// An issue tracker keeps one issue per incident, under the incident's
// thread key (notification.Message.ThreadKey), the key of its persisted
// thread state. A resolve for an incident it has no issue for (kwatch
// restarted and the saved map lacks it, or the issue was never created)
// can only close an issue the tracker finds by searching. That must not
// be silent: the incident may have an issue that nobody closes.

// resolveWithoutMapping reports a resolve sent to a tracker that keeps
// thread state and has none for the job's thread key.
func resolveWithoutMapping(entry *providerEntry, job deliverJob) bool {
	if !job.isResolve() || !skipsPlainMessages(entry.provider) {
		return false
	}
	key := job.incident.ThreadKey()
	if lookup, ok := entry.provider.(ThreadLookup); ok {
		return !lookup.HasThread(key)
	}
	tracker, ok := entry.provider.(ThreadStateProvider)
	if !ok {
		return false
	}
	_, mapped := tracker.SnapshotThreads()[key]
	return !mapped
}

// warnUnmappedResolve logs a resolve that has no issue to close.
func warnUnmappedResolve(entry *providerEntry, job deliverJob) {
	if resolveWithoutMapping(entry, job) {
		klog.Warningf("delivery: resolve for %q (alert key %q) has no "+
			"issue mapped at %s; it is closed only if the tracker "+
			"finds it by its marker",
			job.key(), job.incident.DedupKey, entry.provider.Name())
	}
}
