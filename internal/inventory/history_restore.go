package inventory

import (
	"sort"
	"time"
)

// ChangeRetention is how long the model keeps changes and health marks
// in memory, and how far back a restart reloads them from the state
// file. The rings are bounded by count, so a longer age costs nothing
// on a busy cluster.
const ChangeRetention = 24 * time.Hour

// RestoreChanges puts changes saved by an earlier run back into the
// change history, with their original times and actors. It is meant for
// startup, before the sources report: restored changes are history, not
// observations, so nothing marks the entities touched. It returns how
// many it kept. The rings keep their usual bounds.
func (m *Model) RestoreChanges(changes []Change) int {
	sorted := append([]Change(nil), changes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].At.Before(sorted[j].At)
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := 0
	for _, change := range sorted {
		if change.Entity.IsZero() || change.At.IsZero() {
			continue
		}
		m.change(Observation{
			Entity: change.Entity, At: change.At, Change: change,
		})
		kept++
	}
	return kept
}

// RestoreHealth puts a health mark saved by an earlier run back, so the
// last healthy time of an entity survives a restart.
func (m *Model) RestoreHealth(id EntityID, mark HealthMark) {
	if id.IsZero() || mark.At.IsZero() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.recordFor(id)
	rec.health = append(rec.health, mark)
	sort.SliceStable(rec.health, func(i, j int) bool {
		return rec.health[i].At.Before(rec.health[j].At)
	})
	if over := len(rec.health) - DefaultMaxHealthMarks; over > 0 {
		rec.health = append(rec.health[:0:0], rec.health[over:]...)
	}
}
