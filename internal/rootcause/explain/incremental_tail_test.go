package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Dropping areas must not leave them referenced past the kept slice,
// where the collector could not free them.
func TestSetAreasClearsDroppedTail(t *testing.T) {
	id := inventory.EntityID{Kind: "pod", Name: "a"}
	sv := &Solver{areas: []solvedArea{
		{inputs: map[inventory.EntityID]bool{id: true}},
		{inputs: map[inventory.EntityID]bool{id: true}},
	}}
	full := sv.areas
	sv.setAreas(sv.areas[:1])
	if full[1].inputs != nil {
		t.Fatal("dropped area is still referenced from the slice tail")
	}
	if len(sv.areas) != 1 || sv.areas[0].inputs == nil {
		t.Fatal("kept area was disturbed")
	}
}
