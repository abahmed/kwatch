package main

import "testing"

func TestEnvIntUsesFallbackForInvalidValues(t *testing.T) {
	t.Setenv("MEMORY_MB", "invalid")
	if got := envInt("MEMORY_MB", 7); got != 7 {
		t.Fatalf("got %d, want fallback 7", got)
	}
}

func TestEnvIntAcceptsPositiveValues(t *testing.T) {
	t.Setenv("MEMORY_MB", "9")
	if got := envInt("MEMORY_MB", 7); got != 9 {
		t.Fatalf("got %d, want 9", got)
	}
}
