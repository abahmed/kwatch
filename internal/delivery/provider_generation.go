package delivery

import (
	"k8s.io/klog/v2"
)

type generationState string

const (
	generationAccepting generationState = "accepting"
	generationDraining  generationState = "draining"
	generationStopped   generationState = "stopped"
	generationFailed    generationState = "failed"
)

// providerGeneration is an immutable lookup snapshot for one configured
// provider set. Entries are values so a reconfiguration cannot invalidate a
// fallback while an older delivery is still finishing.
type providerGeneration struct {
	entries map[string]providerEntry
	order   []string
	state   generationState
}

func newProviderGeneration(entries []providerEntry) *providerGeneration {
	generation := &providerGeneration{
		entries: make(map[string]providerEntry, len(entries)),
		order:   make([]string, 0, len(entries)),
		state:   generationAccepting,
	}
	for _, entry := range entries {
		name := entry.lookupName()
		if _, exists := generation.entries[name]; exists {
			klog.InfoS(
				"duplicate provider entry ignored",
				"provider", entry.provider.Name(),
			)
			continue
		}
		generation.entries[name] = entry
		generation.order = append(generation.order, name)
	}
	return generation
}

// cloneProviderGeneration copies a generation. With newChannels every
// entry gets a fresh queue, which Start needs for the workers it launches.
func cloneProviderGeneration(
	generation *providerGeneration,
	newChannels bool,
) *providerGeneration {
	if generation == nil {
		return nil
	}
	clone := &providerGeneration{
		entries: make(map[string]providerEntry, len(generation.entries)),
		order:   append([]string(nil), generation.order...),
		state:   generation.state,
	}
	for name, entry := range generation.entries {
		if newChannels {
			entry.ch = make(chan deliverJob, channelCap)
		}
		clone.entries[name] = entry
	}
	return clone
}

func (m *Manager) currentGenerationLocked() *providerGeneration {
	return m.generation
}
