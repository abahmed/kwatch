package feature

import "testing"

func TestTierOf(t *testing.T) {
	community := []ID{
		PodDetection, PodScheduling, PodOOM, PodRestarts, NodeDetection,
		RootCause, NoiseControl, Scope, RBACAudit,
	}
	want := map[ID]Tier{}
	for _, id := range community {
		want[id] = Community
	}
	for _, definition := range Catalog() {
		expected := Pro
		if want[definition.ID] == Community {
			expected = Community
		}
		if got := TierOf(definition.ID); got != expected {
			t.Errorf("TierOf(%q) = %q, want %q", definition.ID, got, expected)
		}
	}
	if len(tiers) != len(community) {
		t.Errorf("tiers has %d entries, want %d", len(tiers), len(community))
	}
	if got := TierOf(ID("missing.feature")); got != Pro {
		t.Errorf("TierOf(unknown) = %q, want pro", got)
	}
}
