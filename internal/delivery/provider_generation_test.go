package delivery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderGenerationResolvesFallbackByStableName(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary"}
	fallback := &errorRecorderProvider{name: "Fallback"}
	entries := []providerEntry{
		{
			provider:     primary,
			fallbackName: "fallback",
		},
		{provider: fallback},
	}
	manager := Manager{generation: newProviderGeneration(entries)}

	// Reallocation of the compatibility slice must not change name-based
	// fallback resolution for an in-flight delivery.
	entries = append(entries, providerEntry{
		provider: &errorRecorderProvider{name: "Later"},
	})
	setManagerEntries(&manager, entries)
	resolved, ok := manager.fallbackFor("fallback", nil)
	require.True(t, ok)
	require.Same(t, fallback, resolved.provider)
}

func TestFallbackForUsesJobGeneration(t *testing.T) {
	oldFallback := &errorRecorderProvider{name: "Fallback"}
	newFallback := &errorRecorderProvider{name: "Fallback"}
	oldGeneration := newProviderGeneration([]providerEntry{{
		provider: oldFallback,
	}})
	manager := Manager{
		generation: newProviderGeneration([]providerEntry{{
			provider: newFallback,
		}}),
	}

	resolved, ok := manager.fallbackFor("fallback", oldGeneration)
	require.True(t, ok)
	require.Same(t, oldFallback, resolved.provider)
}

func TestSanitizeFallbackCyclesDisablesClosingFallback(t *testing.T) {
	entries := []providerEntry{
		{provider: &errorRecorderProvider{name: "primary"}, fallbackName: "backup"},
		{provider: &errorRecorderProvider{name: "backup"}, fallbackName: "primary"},
	}

	disabled := sanitizeFallbackCycles(entries)

	require.Equal(t, []string{"primary"}, disabled)
	require.Empty(t, entries[0].fallbackName)
	require.Equal(t, "primary", entries[1].fallbackName)
}

func TestProviderGenerationPreservesDeterministicOrder(t *testing.T) {
	entries := []providerEntry{
		{provider: &errorRecorderProvider{name: "zulu"}},
		{provider: &errorRecorderProvider{name: "Alpha"}},
		{provider: &errorRecorderProvider{name: "bravo"}},
	}

	generation := newProviderGeneration(entries)

	require.Equal(t, []string{"zulu", "alpha", "bravo"}, generation.order)
}

func TestProviderGenerationIgnoresDuplicateNames(t *testing.T) {
	first := &errorRecorderProvider{name: "duplicate"}
	second := &errorRecorderProvider{name: "DUPLICATE"}

	generation := newProviderGeneration([]providerEntry{
		{provider: first}, {provider: second},
	})

	require.Equal(t, []string{"duplicate"}, generation.order)
	require.Same(t, first, generation.entries["duplicate"].provider)
}
