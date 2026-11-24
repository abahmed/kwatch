package scenarios

import (
	"slices"
	"testing"
)

// TestStagingDayHasRoomForTheLibrary fails before the library fills the
// staging day: stagingSlots must grow first, which moves every scenario
// once.
func TestStagingDayHasRoomForTheLibrary(t *testing.T) {
	n := len(stagingScenarios())
	if n > stagingRoom {
		t.Fatalf("%d scenarios in the staging day, room for %d: raise "+
			"stagingSlots", n, stagingRoom)
	}
}

// TestStagingSlotsDependOnTheNameOnly: a scenario added to a free slot
// moves no other scenario, and one removed that probes no shared part
// moves none that sat in the slot its name hashes to.
func TestStagingSlotsDependOnTheNameOnly(t *testing.T) {
	places := stagingPlaces(stagingScenarios())
	before := stagingSlotsOf(places)
	used := map[int]bool{}
	for _, slot := range before {
		used[slot] = true
	}
	added := stagingPlace{name: "added-later"}
	for used[homeSlot(added.name)] {
		added.name += "-x"
	}
	after := stagingSlotsOf(append(slices.Clone(places), added))
	gone := slices.IndexFunc(places,
		func(p stagingPlace) bool { return len(p.shared) == 0 })
	shorter := stagingSlotsOf(slices.Delete(slices.Clone(places),
		gone, gone+1))
	for i, p := range places {
		if before[p.name] != after[p.name] {
			t.Errorf("adding %s moved %s from slot %d to %d",
				added.name, p.name, before[p.name], after[p.name])
		}
		if i != gone && homeSlot(p.name) == before[p.name] &&
			shorter[p.name] != before[p.name] {
			t.Errorf("removing %s moved %s from its own slot %d",
				places[gone].name, p.name, before[p.name])
		}
	}
}

// TestStagingDayKeepsSharedPartsApart: two scenarios that fail the same
// shared part of the cluster are at least stagingApart slots apart.
func TestStagingDayKeepsSharedPartsApart(t *testing.T) {
	places := stagingPlaces(stagingScenarios())
	slots := stagingSlotsOf(places)
	for i, a := range places {
		for _, b := range places[i+1:] {
			gap := max(slots[a.name]-slots[b.name],
				slots[b.name]-slots[a.name])
			if gap < stagingApart && sharesPart(a, b) {
				t.Errorf("%s and %s fail one part %d slots apart",
					a.name, b.name, gap)
			}
		}
	}
}

func sharesPart(a, b stagingPlace) bool {
	for _, part := range a.shared {
		if slices.Contains(b.shared, part) {
			return true
		}
	}
	return false
}
