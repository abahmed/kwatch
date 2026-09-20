package delivery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeFallbackCyclesDisablesThreeNodeCycle(t *testing.T) {
	entries := []providerEntry{
		{provider: &errorRecorderProvider{
			name: "alpha",
		}, fallbackName: "bravo"},
		{provider: &errorRecorderProvider{
			name: "bravo",
		}, fallbackName: "charlie"},
		{provider: &errorRecorderProvider{
			name: "charlie",
		}, fallbackName: "alpha"},
	}

	disabled := sanitizeFallbackCycles(entries)

	require.Equal(t, []string{"alpha"}, disabled)
	require.Empty(t, entries[0].fallbackName)
	require.Equal(t, "charlie", entries[1].fallbackName)
	require.Equal(t, "alpha", entries[2].fallbackName)
}

func TestSanitizeFallbackCyclesLeavesMissingTargetBounded(t *testing.T) {
	entries := []providerEntry{{
		provider:     &errorRecorderProvider{name: "primary"},
		fallbackName: "missing",
	}}

	disabled := sanitizeFallbackCycles(entries)

	require.Empty(t, disabled)
	require.Equal(t, "missing", entries[0].fallbackName)
}
