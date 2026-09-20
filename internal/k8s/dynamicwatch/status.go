package dynamicwatch

// Status summarizes the optional watcher without exposing informer objects.
type Status struct {
	State            string   `json:"state"`
	Generation       uint64   `json:"generation,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	InformerCount    int      `json:"informerCount"`
	Synced           int      `json:"synced"`
	Unsynced         int      `json:"unsynced"`
	Skipped          int      `json:"skipped"`
	SkippedResources []string `json:"skippedResources,omitempty"`
}

// Status returns synchronization state suitable for a health adapter.
func (w *Watcher) Status() Status {
	if w == nil {
		return Status{State: "unavailable"}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	status := Status{
		State: "unavailable", Generation: w.generation,
		InformerCount: len(w.informers),
		Skipped:       len(w.skippedGVR),
	}
	for _, gvr := range w.skippedGVR {
		status.SkippedResources = append(
			status.SkippedResources, gvr.String(),
		)
	}
	for _, informer := range w.informers {
		if informer.HasSynced() {
			status.Synced++
		} else {
			status.Unsynced++
		}
	}
	switch {
	case status.Unsynced > 0:
		status.State, status.Reason = "partial", "cache_sync_pending"
	case status.InformerCount > 0:
		status.State = "healthy"
	case status.Skipped > 0:
		status.State, status.Reason = "degraded", "optional_api_unavailable"
	}
	return status
}
