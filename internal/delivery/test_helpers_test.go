package delivery

// managerWithEntries builds a delivery manager fixture from one immutable
// provider generation. Tests should exercise the same representation used by
// production instead of mutating an obsolete provider slice.
func managerWithEntries(entries []providerEntry) *Manager {
	return &Manager{generation: newProviderGeneration(entries)}
}

func setManagerEntries(manager *Manager, entries []providerEntry) {
	manager.generation = newProviderGeneration(entries)
}

func appendManagerEntries(manager *Manager, entries ...providerEntry) {
	current := generationEntries(manager.generation)
	setManagerEntries(manager, append(current, entries...))
}

func managerEntries(manager *Manager) []providerEntry {
	return generationEntries(manager.generation)
}
