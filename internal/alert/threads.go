package alert

// A restart used to orphan every open incident's Slack thread. The incident
// itself survived in the ConfigMap, so the resolve went out -- as a brand new
// top-level message, while the original alert sat above it with no closing
// reply. Anyone scrolling the channel saw a problem that was never closed.
//
// The thread ids are provider state, not incident state, so they persist
// beside the incidents rather than inside them: the map is keyed by provider
// name, and a provider that no longer exists simply has its entry ignored.

// ThreadStateProvider is an optional interface for providers that keep
// per-incident conversation ids worth surviving a restart.
type ThreadStateProvider interface {
	// SnapshotThreads returns incident key → provider thread id.
	SnapshotThreads() map[string]string
	// RestoreThreads adopts a previously saved map. Implementations must
	// tolerate keys for incidents that no longer exist; those entries expire
	// with the provider's own bound.
	RestoreThreads(map[string]string)
}

// SnapshotThreads collects thread state from every provider that keeps it.
// Nil when no provider does, so an empty map is never written.
func (a *AlertManager) SnapshotThreads() map[string]map[string]string {
	a.mu.Lock()
	entries := make([]Provider, 0, len(a.entries))
	for i := range a.entries {
		entries = append(entries, a.entries[i].provider)
	}
	a.mu.Unlock()

	var out map[string]map[string]string
	for _, p := range entries {
		tp, ok := p.(ThreadStateProvider)
		if !ok {
			continue
		}
		threads := tp.SnapshotThreads()
		if len(threads) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string]map[string]string)
		}
		out[p.Name()] = threads
	}
	return out
}

// RestoreThreads hands each provider back its own saved threads.
func (a *AlertManager) RestoreThreads(saved map[string]map[string]string) {
	if len(saved) == 0 {
		return
	}
	a.mu.Lock()
	entries := make([]Provider, 0, len(a.entries))
	for i := range a.entries {
		entries = append(entries, a.entries[i].provider)
	}
	a.mu.Unlock()

	for _, p := range entries {
		threads, ok := saved[p.Name()]
		if !ok || len(threads) == 0 {
			continue
		}
		if tp, ok := p.(ThreadStateProvider); ok {
			tp.RestoreThreads(threads)
		}
	}
}
