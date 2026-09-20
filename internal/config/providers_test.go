package config

import (
	"sort"
	"testing"
)

func TestIsKnownProviderNormalizesName(t *testing.T) {
	if !IsKnownProvider("Slack") {
		t.Fatal("expected provider lookup to be case-insensitive")
	}
	if IsKnownProvider("not-a-provider") {
		t.Fatal("unexpected unknown provider match")
	}
}

func TestKnownProviderNamesReturnsSortedCopy(t *testing.T) {
	names := KnownProviderNames()
	if len(names) == 0 {
		t.Fatal("expected known providers")
	}
	if !sort.StringsAreSorted(names) {
		t.Fatal("expected provider names to be sorted")
	}

	names[0] = "changed"
	if KnownProviderNames()[0] == "changed" {
		t.Fatal("provider names leaked mutable registry state")
	}
}
