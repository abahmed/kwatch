package pipeline

import "github.com/abahmed/kwatch/internal/inventory"

// withoutRestored drops the downtime observations that a change restored
// from the previous run already explains. A change made just before a
// crash is in the saved changes with its real actor and time, but the
// last fingerprint snapshot is older, so the fingerprint comparison finds
// it too and could only say "changed while kwatch was down". The
// restored change wins when it is of the same class, on the same entity,
// and made at or after the snapshot the fingerprint was taken at.
func withoutRestored(
	model inventory.Reader, observations []inventory.Observation,
) []inventory.Observation {
	kept := observations[:0:0]
	for _, observation := range observations {
		if !restoredCovers(model, observation) {
			kept = append(kept, observation)
		}
	}
	return kept
}

func restoredCovers(
	model inventory.Reader, observation inventory.Observation,
) bool {
	snapshot := observation.Change.At
	found := observation.Change
	found.Entity = observation.Entity
	for _, real := range model.Changes(observation.Entity, snapshot) {
		if real.Actor != DowntimeActor &&
			real.Classify() == found.Classify() {
			return true
		}
	}
	return false
}
