package incident

// markProcessingStarted freezes post-construction dependency wiring. The
// engine's state machine may be called by several monitor goroutines, so the
// marker uses the same mutex as lifecycle state.
func (e *Engine) markProcessingStarted() {
	e.mu.Lock()
	e.processingStarted = true
	e.mu.Unlock()
}
