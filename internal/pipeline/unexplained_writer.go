package pipeline

import "encoding/json"

// addUnexplained queues the encoded unexplained counts. A newer value
// replaces the queued one, so a slow store never builds a backlog.
func (w *historyWriter) addUnexplained(raw json.RawMessage) {
	w.mu.Lock()
	w.unexplained = raw
	w.mu.Unlock()
	notify(w.wake)
}

// flushUnexplained writes the queued counts. A failed write is kept,
// unless a newer value came in meanwhile. It reports false on failure.
func (w *historyWriter) flushUnexplained() bool {
	store, ok := w.store.(unexplainedStore)
	w.mu.Lock()
	raw := w.unexplained
	w.unexplained = nil
	w.mu.Unlock()
	if !ok || raw == nil {
		return true
	}
	if err := store.SaveUnexplained(raw); err != nil {
		w.failed(err, "unexplained counts")
		w.mu.Lock()
		if w.unexplained == nil {
			w.unexplained = raw
		}
		w.mu.Unlock()
		return false
	}
	return true
}

// saveUnexplained hands the counts to the history writer, which exists
// only with a store that keeps history.
func (h *historyRecorder) saveUnexplained(raw json.RawMessage) {
	if h.writer != nil {
		h.writer.addUnexplained(raw)
	}
}
