package delivery

// Thread ids must survive a restart. Otherwise the next update of every
// open incident opens a new Slack thread or a new tracker issue while the
// original sits above it with no closing reply.
//
// Thread ids are provider state, not problem state. The application saves
// them in the disk store (internal/app/thread_state.go) keyed by provider
// name and restores them before delivery starts; a provider that no longer
// exists simply has its entry ignored.

// ThreadLookup is an optional cheap per-key check next to
// ThreadStateProvider: HasThread reports whether the provider tracks key,
// even an incident it created but could not address (not persisted).
type ThreadLookup interface {
	HasThread(key string) bool
}

// ThreadStateProvider is an optional interface for providers that keep
// per-problem conversation ids worth surviving a restart.
type ThreadStateProvider interface {
	// SnapshotThreads returns problem key → provider thread id.
	SnapshotThreads() map[string]string
	// RestoreThreads adopts a previously saved map. Implementations must
	// tolerate keys for problems that no longer exist; those entries expire
	// with the provider's own bound.
	RestoreThreads(map[string]string)
}

// SnapshotThreads collects thread state from every provider that keeps it.
// Nil when no provider does, so an empty map is never written.
func (m *Manager) SnapshotThreads() map[string]map[string]string {
	m.mu.Lock()
	generation := cloneProviderGeneration(
		m.currentGenerationLocked(), false,
	)
	entries := make([]Provider, 0)
	if generation != nil {
		entries = make([]Provider, 0, len(generation.order))
		for _, name := range generation.order {
			entries = append(entries, generation.entries[name].provider)
		}
	}
	m.mu.Unlock()

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
func (m *Manager) RestoreThreads(saved map[string]map[string]string) {
	if len(saved) == 0 {
		return
	}
	m.mu.Lock()
	generation := cloneProviderGeneration(
		m.currentGenerationLocked(), false,
	)
	entries := make([]Provider, 0)
	if generation != nil {
		entries = make([]Provider, 0, len(generation.order))
		for _, name := range generation.order {
			entries = append(entries, generation.entries[name].provider)
		}
	}
	m.mu.Unlock()

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
