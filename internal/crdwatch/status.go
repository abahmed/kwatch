package crdwatch

import "strings"

// Status describes CRD discovery and informer health without exposing the
// watcher's internal clients or lifecycle handles.
type Status struct {
	State          string `json:"state"`
	WaitingForCRD  bool   `json:"waitingForCRD"`
	Ready          bool   `json:"ready"`
	RestartRequest bool   `json:"restartRequested"`
	LastError      string `json:"lastError,omitempty"`
}

// Status returns the current CRD watcher state for diagnostics.
func (w *Watcher) Status() Status {
	if w == nil {
		return Status{State: "unavailable"}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	state := "stopped"
	if w.started {
		state = "running"
	}
	if w.started && !w.ready && w.lastError == "" {
		state = "waiting"
	}
	if w.lastError != "" {
		state = "degraded"
	}
	return Status{
		State:          state,
		WaitingForCRD:  w.started && !w.ready,
		Ready:          w.ready,
		RestartRequest: w.restartRequested,
		LastError:      w.lastError,
	}
}

func (w *Watcher) reportError(err error) {
	w.mu.Lock()
	sink := w.statusSink
	stateSink := w.stateSink
	if err == nil {
		w.lastError = ""
	} else {
		w.lastError = safeWatcherReason(err.Error())
	}
	w.mu.Unlock()
	if sink != nil {
		sink(err)
	}
	if stateSink != nil {
		stateSink(w.Status())
	}
}

func safeWatcherReason(message string) string {
	message = strings.ToLower(message)
	switch {
	case strings.Contains(message, "sync"):
		return "cache_sync_failed"
	case strings.Contains(message, "not configured"):
		return "source_not_configured"
	case strings.Contains(message, "not found"):
		return "optional_api_unavailable"
	default:
		return "watcher_failed"
	}
}
