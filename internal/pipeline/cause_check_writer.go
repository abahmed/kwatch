package pipeline

import "encoding/json"

// addChecks queues the encoded cause self-check counts. A newer value
// replaces the queued one, so a slow store never builds a backlog.
func (w *historyWriter) addChecks(raw json.RawMessage) {
	w.mu.Lock()
	w.checks = raw
	w.mu.Unlock()
	notify(w.wake)
}

// flushChecks writes the queued counts. A failed write is kept, unless a
// newer value came in meanwhile. It reports false on failure.
func (w *historyWriter) flushChecks() bool {
	store, ok := w.store.(causeStore)
	w.mu.Lock()
	raw := w.checks
	w.checks = nil
	w.mu.Unlock()
	if !ok || raw == nil {
		return true
	}
	if err := store.SaveCauseChecks(raw); err != nil {
		w.failed(err, "cause checks")
		w.mu.Lock()
		if w.checks == nil {
			w.checks = raw
		}
		w.mu.Unlock()
		return false
	}
	return true
}

// saveChecks hands the counts to the history writer, which exists only
// with a store that keeps history.
func (h *historyRecorder) saveChecks(raw json.RawMessage) {
	if h.writer != nil {
		h.writer.addChecks(raw)
	}
}
