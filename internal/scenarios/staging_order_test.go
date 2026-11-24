package scenarios

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
)

// stagingSlots is how many slots of stagingSpacing the staging day has.
// It is fixed, so the day and every scenario's place in it stay the same
// when the library grows; TestStagingDayHasRoomForTheLibrary fails
// before the library outgrows it.
const stagingSlots = 1024

// stagingRoom is how many slots the library may fill. The rest keeps
// the runs of taken slots short, so a new scenario rarely lands in the
// slot another one would have moved on to.
const stagingRoom = stagingSlots * 95 / 100

// stagingApart is how many slots (three hours) lie at least between two
// scenarios that fail the same shared part of the cluster, one kwatch
// probes (its DNS, its scheduler, its API server): the first is fixed,
// resolved and past the incident's ReopenWindow before the next begins.
// Unrelated outages of one part come at unrelated hours; two at once
// would be one outage.
const stagingApart = 20

// stagingPlace is what decides a scenario's slot: its name, and the
// shared parts of the cluster it probes.
type stagingPlace struct {
	name   string
	shared []string
}

// stagingPlaces lists the place of every scenario.
func stagingPlaces(scenarios []scenario) []stagingPlace {
	out := make([]stagingPlace, 0, len(scenarios))
	for _, s := range scenarios {
		c := newCluster(stagingStart, "")
		s.build(c)
		var shared []string
		for _, id := range c.probed {
			shared = append(shared, id.String())
		}
		slices.Sort(shared)
		out = append(out, stagingPlace{
			name: s.expect.Name, shared: slices.Compact(shared),
		})
	}
	return out
}

// stagingSlotsOf places the scenarios of the staging day. In name order,
// each takes the slot its name hashes to or, when that slot is taken or
// within stagingApart of a scenario that probes a part it probes too,
// the next one that is neither. A scenario's time so depends on its own
// name and not on the rest of the library: adding or removing a
// scenario moves another only when they want the same slot or fail the
// same shared part.
func stagingSlotsOf(places []stagingPlace) map[string]int {
	if len(places) > stagingSlots {
		panic(fmt.Sprintf("staging day: %d scenarios for %d slots",
			len(places), stagingSlots))
	}
	sorted := slices.SortedFunc(slices.Values(places),
		func(a, b stagingPlace) int { return strings.Compare(a.name, b.name) })
	taken := make([]bool, stagingSlots)
	near := map[string][]int{}
	out := make(map[string]int, len(places))
	for _, p := range sorted {
		slot := homeSlot(p.name)
		for tries := 0; taken[slot] || tooNear(near, p.shared, slot); tries++ {
			if tries == stagingSlots {
				panic("staging day: no slot for " + p.name)
			}
			slot = (slot + 1) % stagingSlots
		}
		taken[slot] = true
		out[p.name] = slot
		for _, part := range p.shared {
			near[part] = append(near[part], slot)
		}
	}
	return out
}

// tooNear reports a slot within stagingApart of a scenario placed before
// that probes one of the shared parts.
func tooNear(near map[string][]int, shared []string, slot int) bool {
	for _, part := range shared {
		for _, other := range near[part] {
			if max(slot-other, other-slot) < stagingApart {
				return true
			}
		}
	}
	return false
}

// homeSlot is the slot a scenario's name hashes to. It uses another hash
// than jitter, so a scenario's slot says nothing of its offset in it.
func homeSlot(name string) int {
	sum := sha256.Sum256([]byte(name))
	return int(binary.BigEndian.Uint64(sum[:8]) % stagingSlots)
}
