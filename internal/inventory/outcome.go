package inventory

import "time"

// Outcome is what a change set did to what depends on it.
type Outcome string

// Change set outcomes.
const (
	// OutcomePending means the effect window is still open and nothing
	// downstream failed yet.
	OutcomePending Outcome = "pending"
	// OutcomeHealthy means the window closed without downstream failures.
	OutcomeHealthy Outcome = "healthy"
	// OutcomeDegraded means dependents developed failing findings within
	// the window.
	OutcomeDegraded Outcome = "degraded"
	// OutcomeReverted means the changed fields returned to their previous
	// values and every dependent that failed recovered. A revert that
	// recovers confirms the change as the cause.
	OutcomeReverted Outcome = "reverted"
)

// Effect window bounds.
const (
	MinEffectWindow = 5 * time.Minute
	MaxEffectWindow = 30 * time.Minute
)

// Blast-radius bounds, so one widely used object stays cheap to judge.
const (
	blastDepth = 3
	blastLimit = 256
)

// ChangeOutcome is a change set with its judged outcome.
type ChangeOutcome struct {
	Set     ChangeSet
	Outcome Outcome
	// Window is the effect window: max(5m, 2x the baseline ready time of
	// the changed workloads), capped at 30m.
	Window time.Duration
	// Failed are the dependents that failed within the window.
	Failed []EntityID
	// Reverted is when the revert was seen; zero without one.
	Reverted time.Time
}

// ChangeOutcomes returns the change sets related to id that ended at or
// after since, each with its outcome as of now, oldest first.
func (m *Model) ChangeOutcomes(
	id EntityID, since, now time.Time,
) []ChangeOutcome {
	sets := m.ChangeSets(id, since)
	out := make([]ChangeOutcome, 0, len(sets))
	for _, set := range sets {
		out = append(out, m.OutcomeOf(set, now))
	}
	return out
}

// OutcomeOf judges set as of now.
func (m *Model) OutcomeOf(set ChangeSet, now time.Time) ChangeOutcome {
	window := m.EffectWindow(set)
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := ChangeOutcome{Set: set, Window: window}
	radius := m.blastRadiusLocked(set.Entities())
	failed, recovered := m.downstreamHealthLocked(
		radius, set.Start, set.Start.Add(window))
	result.Failed = failed
	result.Reverted = m.revertTimeLocked(set)
	switch {
	case !result.Reverted.IsZero() && recovered:
		result.Outcome = OutcomeReverted
	case len(failed) > 0:
		result.Outcome = OutcomeDegraded
	case now.Before(set.Start.Add(window)):
		result.Outcome = OutcomePending
	default:
		result.Outcome = OutcomeHealthy
	}
	return result
}

// EffectWindow is max(MinEffectWindow, 2x the slowest median ready time
// among the set's entities), capped at MaxEffectWindow.
func (m *Model) EffectWindow(set ChangeSet) time.Duration {
	window := MinEffectWindow
	for _, entity := range set.Entities() {
		stat, ok := m.Baselines().Stat(entity, MetricReadySeconds)
		if !ok {
			continue
		}
		ready := time.Duration(stat.Quantile(0.5) * float64(time.Second))
		window = max(window, 2*ready)
	}
	return min(window, MaxEffectWindow)
}

// blastRadiusLocked returns the changed entities and what depends on
// them: entities with a relation pointing at them, followed blastDepth
// steps.
func (m *Model) blastRadiusLocked(changed []EntityID) []EntityID {
	radius := append([]EntityID(nil), changed...)
	frontier := changed
	for depth := 0; depth < blastDepth && len(frontier) > 0; depth++ {
		var next []EntityID
		for _, id := range frontier {
			for _, relation := range m.edges.relations(id, Incoming) {
				if len(radius) >= blastLimit {
					return radius
				}
				if !containsID(radius, relation.From) {
					radius = append(radius, relation.From)
					next = append(next, relation.From)
				}
			}
		}
		frontier = next
	}
	return radius
}

// downstreamHealthLocked returns the entities that started failing in
// [from, until], and whether every one of them is healthy again now.
func (m *Model) downstreamHealthLocked(
	radius []EntityID, from, until time.Time,
) ([]EntityID, bool) {
	var failed []EntityID
	recovered := true
	for _, id := range radius {
		marks := m.healthMarksLocked(id, from)
		failedHere := false
		for _, mark := range marks {
			if mark.Failing && !mark.At.After(until) {
				failedHere = true
			}
		}
		if !failedHere {
			continue
		}
		failed = append(failed, id)
		if marks[len(marks)-1].Failing {
			recovered = false
		}
	}
	return failed, recovered
}

// revertTimeLocked returns when every entity the set changed returned to
// its values from before the set, or zero when one did not.
func (m *Model) revertTimeLocked(set ChangeSet) time.Time {
	var latest time.Time
	for _, entity := range set.Entities() {
		at, ok := m.entityRevertLocked(entity, set)
		if !ok {
			return time.Time{}
		}
		latest = latestTime(latest, at)
	}
	return latest
}

// entityRevertLocked checks one entity: every field the set's first
// change of it touched is back at its old value, or its revision is back
// at the revision before the set.
func (m *Model) entityRevertLocked(
	entity EntityID, set ChangeSet,
) (time.Time, bool) {
	rec := m.records[entity]
	if rec == nil {
		return time.Time{}, false
	}
	first, previous := firstChangeOf(rec.changes, entity, set.Start)
	if first < 0 || rec.changes[first].Created ||
		rec.changes[first].Deleted {
		return time.Time{}, false
	}
	original := rec.changes[first]
	current := map[string]string{}
	revision, at := original.Revision, time.Time{}
	for _, change := range rec.changes[first+1:] {
		for _, field := range change.Fields {
			current[field.Path] = field.After
		}
		if change.Revision != "" {
			revision = change.Revision
		}
		at = change.At
	}
	if at.IsZero() {
		return time.Time{}, false
	}
	if fieldsRestored(original.Fields, current) ||
		(previous != "" && revision == previous) {
		return at, true
	}
	return time.Time{}, false
}

// firstChangeOf returns the index of entity's first change at or after
// start and the revision recorded before it.
func firstChangeOf(
	changes []Change, entity EntityID, start time.Time,
) (int, string) {
	previous := ""
	for i, change := range changes {
		if change.Entity != entity {
			continue
		}
		if !change.At.Before(start) {
			return i, previous
		}
		if change.Revision != "" {
			previous = change.Revision
		}
	}
	return -1, ""
}

// fieldsRestored reports whether every field with a known old value is
// back at it. Fields without an old value ("changed") cannot tell.
func fieldsRestored(original []FieldChange, current map[string]string) bool {
	known := 0
	for _, field := range original {
		if field.Before == "" || field.Before == field.After {
			continue
		}
		known++
		if value, ok := current[field.Path]; !ok || value != field.Before {
			return false
		}
	}
	return known > 0
}

func latestTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
