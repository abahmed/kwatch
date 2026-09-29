package delivery

// A restart used to orphan every open problem's provider thread: the next
// update opened a new Slack thread or a new tracker issue while the original
// sat above it with no closing reply.
//
// Thread ids are provider state, not incident state. The application saves
// them in the disk store (internal/app/thread_state.go) keyed by provider
// name and restores them before delivery starts; a provider that no longer
// exists simply has its entry ignored.

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
func (a *Manager) SnapshotThreads() map[string]map[string]string {
	a.mu.Lock()
	generation := cloneProviderGeneration(
		a.currentGenerationLocked(), false,
	)
	entries := make([]Provider, 0)
	if generation != nil {
		entries = make([]Provider, 0, len(generation.order))
		for _, name := range generation.order {
			entries = append(entries, generation.entries[name].provider)
		}
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
func (a *Manager) RestoreThreads(saved map[string]map[string]string) {
	if len(saved) == 0 {
		return
	}
	a.mu.Lock()
	generation := cloneProviderGeneration(
		a.currentGenerationLocked(), false,
	)
	entries := make([]Provider, 0)
	if generation != nil {
		entries = make([]Provider, 0, len(generation.order))
		for _, name := range generation.order {
			entries = append(entries, generation.entries[name].provider)
		}
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
