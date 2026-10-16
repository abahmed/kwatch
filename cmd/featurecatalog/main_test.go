package main

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/feature"
)

func TestRender(t *testing.T) {
	lines := strings.Split(strings.TrimSuffix(render(), "\n"), "\n")
	if lines[0] != "# kwatch feature catalog v2" {
		t.Fatalf("header = %q", lines[0])
	}
	definitions := feature.Catalog()
	if len(lines)-1 != len(definitions) {
		t.Fatalf("got %d rows, want %d", len(lines)-1, len(definitions))
	}
	for i, definition := range definitions {
		fields := strings.Split(lines[i+1], "|")
		if len(fields) != 5 {
			t.Fatalf("row %q has %d fields, want 5", lines[i+1], len(fields))
		}
		if fields[0] != string(definition.ID) ||
			fields[4] != string(feature.TierOf(definition.ID)) {
			t.Errorf("row %q mismatches %q", lines[i+1], definition.ID)
		}
	}
}
