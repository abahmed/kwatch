package delivery

import (
	"context"
)

// VerifyAll checks every provider and returns the failures by name.
func (m *Manager) VerifyAll(ctx context.Context) map[string]error {
	result := make(map[string]error)
	m.mu.Lock()
	generation := m.currentGenerationLocked()
	providers := make([]Provider, 0)
	if generation != nil {
		providers = make([]Provider, 0, len(generation.order))
		for _, name := range generation.order {
			providers = append(providers,
				generation.entries[name].provider)
		}
	}
	m.mu.Unlock()
	for _, provider := range providers {
		if v, ok := provider.(Verifier); ok {
			result[provider.Name()] = v.Verify(ctx)
		} else {
			result[provider.Name()] = nil // no verifier = skip
		}
	}
	return result
}
